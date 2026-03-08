package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/provider"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
	"github.com/similarityyoung/simiclaw/pkg/tools"
)

func TestProviderRunnerContinuesWhenStreamSinkPanics(t *testing.T) {
	r := newTestRunner(t, config.Default().LLM, nil)
	output, err := r.Run(context.Background(), testEvent("sink panic"), 1, panicSink{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if output.AssistantReply != "已收到: sink panic" {
		t.Fatalf("unexpected assistant reply: %+v", output)
	}
	if output.Trace.Diagnostics["stream_sink.text_delta"] == "" {
		t.Fatalf("expected sink panic diagnostic, got %+v", output.Trace.Diagnostics)
	}
}

func TestProviderRunnerPrependsSystemPrompt(t *testing.T) {
	cfg := config.Default().LLM
	cfg.DefaultModel = "fake/default"
	cfg.Providers["fake"] = config.LLMProviderConfig{
		Type:                 "fake",
		FakeResponseText:     "roles={{message_roles}}\nsystem={{first_system_message}}",
		FakeFinishReason:     "stop",
		FakeRawFinishReason:  "stop",
		FakePromptTokens:     8,
		FakeCompletionTokens: 8,
		FakeRequestID:        "fake-request-1",
	}
	r := newTestRunner(t, cfg, nil)
	output, err := r.Run(context.Background(), testEvent("hello"), 1, nil)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if output.AssistantReply == "" {
		t.Fatalf("expected assistant reply")
	}
	if want := "roles=system,user"; output.AssistantReply[:len(want)] != want {
		t.Fatalf("expected roles prefix %q, got %q", want, output.AssistantReply)
	}
	if !containsAll(output.AssistantReply,
		"## Identity & Runtime Rules",
		"## Project Context",
		"## Available Skills",
		"## Memory Policy",
		"## Current Run Context",
	) {
		t.Fatalf("expected system prompt sections in reply, got %q", output.AssistantReply)
	}
}

func TestProviderRunnerRecoversToolPanicAndStreamsSanitizedToolEvents(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register("panic_tool", tools.Schema{Name: "panic_tool"}, func(context.Context, tools.Context, map[string]any) tools.Result {
		panic("boom")
	})
	cfg := config.Default().LLM
	cfg.DefaultModel = "fake/default"
	cfg.Providers["fake"] = config.LLMProviderConfig{
		Type:                 "fake",
		FakeResponseText:     "已收到: {{last_user_message}}",
		FakeToolName:         "panic_tool",
		FakeToolArgsJSON:     `{"token":"secret","query":"hello"}`,
		FakeFinishReason:     "stop",
		FakeRawFinishReason:  "stop",
		FakePromptTokens:     8,
		FakeCompletionTokens: 8,
		FakeRequestID:        "fake-request-1",
	}
	r := newTestRunner(t, cfg, registry)
	sink := &captureSink{}
	output, err := r.Run(context.Background(), testEvent("tool panic"), 2, sink)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(sink.starts) != 1 || len(sink.results) != 1 {
		t.Fatalf("expected one tool start and one tool result, got starts=%d results=%d", len(sink.starts), len(sink.results))
	}
	if sink.starts[0].Args["token"] != "[redacted]" {
		t.Fatalf("expected redacted token in tool_start, got %+v", sink.starts[0].Args)
	}
	if sink.results[0].Error == nil || sink.results[0].Error.Message != "tool panic: boom" {
		t.Fatalf("expected tool panic error in tool_result, got %+v", sink.results[0])
	}
	if output.AssistantReply != "已收到: tool panic" {
		t.Fatalf("unexpected assistant reply: %+v", output)
	}
	if len(output.Trace.ToolExecutions) != 1 || output.Trace.ToolExecutions[0].Error == nil {
		t.Fatalf("expected tool execution error in trace: %+v", output.Trace.ToolExecutions)
	}
}

func TestProviderRunnerPersistsAssistantToolCallsMessage(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register("echo", tools.Schema{Name: "echo"}, func(_ context.Context, _ tools.Context, args map[string]any) tools.Result {
		return tools.Result{Output: map[string]any{"echo": args["query"]}}
	})
	cfg := config.Default().LLM
	cfg.DefaultModel = "fake/default"
	cfg.Providers["fake"] = config.LLMProviderConfig{
		Type:                 "fake",
		FakeResponseText:     "done: {{last_user_message}}",
		FakeToolName:         "echo",
		FakeToolArgsJSON:     `{"query":"hello"}`,
		FakeFinishReason:     "stop",
		FakeRawFinishReason:  "stop",
		FakePromptTokens:     8,
		FakeCompletionTokens: 8,
		FakeRequestID:        "fake-request-1",
	}
	r := newTestRunner(t, cfg, registry)
	output, err := r.Run(context.Background(), testEvent("remember this"), 2, nil)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	var toolCallMsg *OutputMessage
	for i := range output.Messages {
		msg := &output.Messages[i]
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			toolCallMsg = msg
			break
		}
	}
	if toolCallMsg == nil {
		t.Fatalf("expected hidden assistant tool_calls message, got %+v", output.Messages)
	}
	if toolCallMsg.Visible {
		t.Fatalf("assistant tool_calls message should be hidden, got %+v", toolCallMsg)
	}
	if len(toolCallMsg.ToolCalls) != 1 || toolCallMsg.ToolCalls[0].Name != "echo" {
		t.Fatalf("unexpected tool_calls message: %+v", toolCallMsg)
	}
}

func TestHistoryToChatMessagesSkipsOrphanToolResults(t *testing.T) {
	history := []store.HistoryMessage{
		{Role: "user", Content: "hello"},
		{Role: "tool", Content: `{"path":"AGENTS.md"}`, ToolCallID: "call_orphan", ToolName: "context_get"},
		{Role: "assistant", Content: "world"},
	}

	got := historyToChatMessages(history)
	if len(got) != 2 {
		t.Fatalf("expected orphan tool message to be skipped, got %+v", got)
	}
	if got[0].Role != "user" || got[1].Role != "assistant" {
		t.Fatalf("unexpected chat history order: %+v", got)
	}
}

func TestHistoryToChatMessagesPreservesAssistantToolChain(t *testing.T) {
	history := []store.HistoryMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", ToolCalls: []model.ToolCall{{ToolCallID: "call_1", Name: "memory_search", Args: map[string]any{"query": "hello"}}}},
		{Role: "tool", Content: `{"hits":[]}`, ToolCallID: "call_1", ToolName: "memory_search"},
		{Role: "assistant", Content: "done"},
	}

	got := historyToChatMessages(history)
	if len(got) != 4 {
		t.Fatalf("expected complete tool chain, got %+v", got)
	}
	if got[1].Role != "assistant" || len(got[1].ToolCalls) != 1 || got[1].ToolCalls[0].ToolCallID != "call_1" {
		t.Fatalf("expected assistant tool_calls in history, got %+v", got)
	}
	if got[2].Role != "tool" || got[2].ToolCallID != "call_1" {
		t.Fatalf("expected matched tool result, got %+v", got)
	}
}

func TestProviderRunnerNoReplyWritesCanonicalMemoryPaths(t *testing.T) {
	r, workspace := newTestRunnerWithWorkspace(t, config.Default().LLM, nil)

	dmEvent := model.InternalEvent{
		EventID:         "evt_flush",
		Conversation:    model.Conversation{ConversationID: "conv-dm", ChannelType: "dm", ParticipantID: "u1"},
		SessionKey:      "tenant:dm:u1",
		ActiveSessionID: "ses_flush",
		Payload:         model.EventPayload{Type: "memory_flush", Text: "flush note"},
	}
	if _, err := r.Run(context.Background(), dmEvent, 1, nil); err != nil {
		t.Fatalf("Run memory_flush returned error: %v", err)
	}
	day := time.Now().UTC().Format("2006-01-02")
	privateDaily := filepath.Join(workspace, "memory", "private", "daily", day+".md")
	data, err := os.ReadFile(privateDaily)
	if err != nil {
		t.Fatalf("read private daily: %v", err)
	}
	if !strings.Contains(string(data), "flush note") {
		t.Fatalf("expected flush note in private daily file, got %q", string(data))
	}

	groupEvent := model.InternalEvent{
		EventID:         "evt_compaction",
		Conversation:    model.Conversation{ConversationID: "conv-group", ChannelType: "group"},
		SessionKey:      "tenant:group:1",
		ActiveSessionID: "ses_compaction",
		Payload:         model.EventPayload{Type: "compaction", Text: "group summary"},
	}
	if _, err := r.Run(context.Background(), groupEvent, 1, nil); err != nil {
		t.Fatalf("Run compaction returned error: %v", err)
	}
	publicCurated := filepath.Join(workspace, "memory", "public", "MEMORY.md")
	curated, err := os.ReadFile(publicCurated)
	if err != nil {
		t.Fatalf("read public curated: %v", err)
	}
	if !strings.Contains(string(curated), "group summary") {
		t.Fatalf("expected group summary in public curated file, got %q", string(curated))
	}
}

func newTestRunner(t *testing.T, llm config.LLMConfig, registry *tools.Registry) *ProviderRunner {
	t.Helper()
	workspace := t.TempDir()
	return newTestRunnerAtWorkspace(t, workspace, llm, registry)
}

func newTestRunnerWithWorkspace(t *testing.T, llm config.LLMConfig, registry *tools.Registry) (*ProviderRunner, string) {
	t.Helper()
	workspace := t.TempDir()
	return newTestRunnerAtWorkspace(t, workspace, llm, registry), workspace
}

func newTestRunnerAtWorkspace(t *testing.T, workspace string, llm config.LLMConfig, registry *tools.Registry) *ProviderRunner {
	t.Helper()
	if err := store.InitWorkspace(workspace, false, config.Default().DBBusyTimeout.Duration); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}
	db, err := store.Open(workspace, config.Default().DBBusyTimeout.Duration)
	if err != nil {
		t.Fatalf("Open DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	providers, err := provider.NewFactory(llm)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	return NewProviderRunner(workspace, db, registry, providers)
}

func testEvent(text string) model.InternalEvent {
	return model.InternalEvent{
		EventID:         "evt_test",
		Conversation:    model.Conversation{ConversationID: "conv", ChannelType: "dm", ParticipantID: "u1"},
		SessionKey:      "local:dm:u1",
		ActiveSessionID: "sess_1",
		Payload:         model.EventPayload{Type: "message", Text: text},
	}
}

type panicSink struct{}

func (panicSink) OnStatus(string, string) {}

func (panicSink) OnReasoningDelta(string) {}

func (panicSink) OnTextDelta(string) { panic("sink boom") }

func (panicSink) OnToolStart(string, string, map[string]any, bool) {}

func (panicSink) OnToolResult(string, string, map[string]any, bool, *model.ErrorBlock) {}

type captureSink struct {
	starts  []capturedToolStart
	results []capturedToolResult
}

type capturedToolStart struct {
	ToolCallID string
	ToolName   string
	Args       map[string]any
	Truncated  bool
}

type capturedToolResult struct {
	ToolCallID string
	ToolName   string
	Result     map[string]any
	Truncated  bool
	Error      *model.ErrorBlock
}

func (c *captureSink) OnStatus(string, string) {}

func (c *captureSink) OnReasoningDelta(string) {}

func (c *captureSink) OnTextDelta(string) {}

func (c *captureSink) OnToolStart(toolCallID, toolName string, args map[string]any, truncated bool) {
	c.starts = append(c.starts, capturedToolStart{
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Args:       args,
		Truncated:  truncated,
	})
}

func (c *captureSink) OnToolResult(toolCallID, toolName string, result map[string]any, truncated bool, apiErr *model.ErrorBlock) {
	c.results = append(c.results, capturedToolResult{
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Result:     result,
		Truncated:  truncated,
		Error:      apiErr,
	})
}

func containsAll(in string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(in, needle) {
			return false
		}
	}
	return true
}
