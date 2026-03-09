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
		"## Tool Contract",
		"## Memory Policy",
		"## Workspace Instructions & Context",
		"## Available Skills",
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

func TestProviderRunnerBuiltinsIncludeWorkspaceWriteTools(t *testing.T) {
	r := newTestRunner(t, config.Default().LLM, nil)
	defs := r.registry.Definitions()
	names := map[string]bool{}
	for _, def := range defs {
		names[def.Schema.Name] = true
	}
	if !names["workspace_patch"] || !names["workspace_delete"] {
		t.Fatalf("expected workspace write tools in builtins, got %+v", names)
	}
}

func TestProviderRunnerWorkspacePatchWritesFileAndMarksMediumRisk(t *testing.T) {
	cfg := config.Default().LLM
	cfg.DefaultModel = "fake/default"
	cfg.Providers["fake"] = config.LLMProviderConfig{
		Type:                 "fake",
		FakeResponseText:     "done: {{last_user_message}}",
		FakeToolName:         "workspace_patch",
		FakeToolArgsJSON:     `{"path":"IDENTITY.md","old_text":"SimiClaw","new_text":"Simi 龙虾"}`,
		FakeFinishReason:     "stop",
		FakeRawFinishReason:  "stop",
		FakePromptTokens:     8,
		FakeCompletionTokens: 8,
		FakeRequestID:        "fake-request-1",
	}
	r, workspace := newTestRunnerWithWorkspace(t, cfg, nil)
	if err := os.WriteFile(filepath.Join(workspace, "IDENTITY.md"), []byte("- Name: SimiClaw\n"), 0o644); err != nil {
		t.Fatalf("write IDENTITY.md: %v", err)
	}

	output, err := r.Run(context.Background(), testEvent("rename yourself"), 2, nil)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "IDENTITY.md"))
	if err != nil {
		t.Fatalf("read IDENTITY.md: %v", err)
	}
	if !strings.Contains(string(data), "Simi 龙虾") {
		t.Fatalf("expected patched file, got %q", string(data))
	}
	if len(output.Trace.ToolExecutions) != 1 || output.Trace.ToolExecutions[0].Name != "workspace_patch" {
		t.Fatalf("expected workspace_patch execution, got %+v", output.Trace.ToolExecutions)
	}
	if len(output.Trace.Actions) != 1 || output.Trace.Actions[0].Risk != "medium" {
		t.Fatalf("expected medium-risk tool action, got %+v", output.Trace.Actions)
	}
	if output.Messages[2].ToolName != "workspace_patch" {
		t.Fatalf("expected persisted tool message, got %+v", output.Messages)
	}
}

func TestProviderRunnerWorkspaceDeleteMarksHighRisk(t *testing.T) {
	cfg := config.Default().LLM
	cfg.DefaultModel = "fake/default"
	cfg.Providers["fake"] = config.LLMProviderConfig{
		Type:                 "fake",
		FakeResponseText:     "done: {{last_user_message}}",
		FakeToolName:         "workspace_delete",
		FakeToolArgsJSON:     `{"path":"BOOTSTRAP.md"}`,
		FakeFinishReason:     "stop",
		FakeRawFinishReason:  "stop",
		FakePromptTokens:     8,
		FakeCompletionTokens: 8,
		FakeRequestID:        "fake-request-1",
	}
	r, workspace := newTestRunnerWithWorkspace(t, cfg, nil)
	if err := os.WriteFile(filepath.Join(workspace, "BOOTSTRAP.md"), []byte("cleanup me\n"), 0o644); err != nil {
		t.Fatalf("write BOOTSTRAP.md: %v", err)
	}

	output, err := r.Run(context.Background(), testEvent("cleanup bootstrap"), 2, nil)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "BOOTSTRAP.md")); !os.IsNotExist(err) {
		t.Fatalf("expected BOOTSTRAP.md deleted, err=%v", err)
	}
	if len(output.Trace.Actions) != 1 || output.Trace.Actions[0].Risk != "high" {
		t.Fatalf("expected high-risk tool action, got %+v", output.Trace.Actions)
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

func TestHistoryToChatMessagesSkipsCronFireHiddenMessages(t *testing.T) {
	history := []store.HistoryMessage{
		{Role: "user", Content: "nightly tick", Meta: map[string]any{"payload_type": "cron_fire"}},
		{Role: "assistant", ToolCalls: []model.ToolCall{{ToolCallID: "call_1", Name: "memory_search", Args: map[string]any{"query": "nightly tick"}}}, Meta: map[string]any{"payload_type": "cron_fire"}},
		{Role: "tool", Content: `{"hits":[]}`, ToolCallID: "call_1", ToolName: "memory_search", Meta: map[string]any{"payload_type": "cron_fire"}},
		{Role: "assistant", Content: "done", Meta: map[string]any{"payload_type": "cron_fire"}},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}

	got := historyToChatMessages(history)
	if len(got) != 2 {
		t.Fatalf("expected cron_fire hidden history to be skipped, got %+v", got)
	}
	if got[0].Role != "user" || got[0].Content != "hello" {
		t.Fatalf("expected normal user message to remain, got %+v", got)
	}
	if got[1].Role != "assistant" || got[1].Content != "world" {
		t.Fatalf("expected normal assistant message to remain, got %+v", got)
	}
}

func TestProviderRunnerCronFireRunsSuppressedLLMWithHiddenMessages(t *testing.T) {
	cfg := config.Default().LLM
	cfg.DefaultModel = "fake/default"
	cfg.Providers["fake"] = config.LLMProviderConfig{
		Type:                 "fake",
		FakeResponseText:     "roles={{message_roles}} last={{last_user_message}}",
		FakeToolName:         "memory_search",
		FakeToolArgsJSON:     `{"query":"alpha","visibility":"auto","kind":"any","top_k":1}`,
		FakeFinishReason:     "stop",
		FakeRawFinishReason:  "stop",
		FakePromptTokens:     8,
		FakeCompletionTokens: 8,
		FakeRequestID:        "fake-request-1",
	}
	r, workspace := newTestRunnerWithWorkspace(t, cfg, nil)
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "public"), 0o755); err != nil {
		t.Fatalf("mkdir memory/public: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "memory", "public", "MEMORY.md"), []byte("alpha memory\n"), 0o644); err != nil {
		t.Fatalf("write public memory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "HEARTBEAT.md"), []byte("- check memory\n"), 0o644); err != nil {
		t.Fatalf("write HEARTBEAT.md: %v", err)
	}

	output, err := r.Run(context.Background(), model.InternalEvent{
		EventID:         "evt_cron",
		Conversation:    model.Conversation{ConversationID: "conv-cron", ChannelType: "dm", ParticipantID: "u1"},
		SessionKey:      "local:dm:u1",
		ActiveSessionID: "sess_cron",
		Payload:         model.EventPayload{Type: "cron_fire", Text: "nightly heartbeat"},
	}, 2, nil)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if output.RunMode != model.RunModeNoReply {
		t.Fatalf("expected NO_REPLY run mode, got %+v", output)
	}
	if !output.SuppressOutput {
		t.Fatalf("expected suppressed output, got %+v", output)
	}
	if output.AssistantReply != "" {
		t.Fatalf("expected empty assistant reply, got %+v", output)
	}
	if !strings.Contains(output.Trace.OutputText, "roles=system,user,assistant,tool last=nightly heartbeat") {
		t.Fatalf("expected second-round fake response in trace output, got %+v", output.Trace)
	}
	if len(output.Trace.ToolExecutions) != 1 {
		t.Fatalf("expected one tool execution, got %+v", output.Trace.ToolExecutions)
	}
	if output.Trace.ToolExecutions[0].Name != "memory_search" || output.Trace.ToolExecutions[0].Error != nil {
		t.Fatalf("expected allowed memory_search execution, got %+v", output.Trace.ToolExecutions)
	}
	if len(output.Messages) < 4 {
		t.Fatalf("expected hidden user/tool/final assistant messages, got %+v", output.Messages)
	}
	for _, msg := range output.Messages {
		if msg.Visible {
			t.Fatalf("expected all cron_fire messages hidden, got %+v", output.Messages)
		}
		if msg.Meta["payload_type"] != "cron_fire" {
			t.Fatalf("expected cron_fire payload_type meta on all messages, got %+v", output.Messages)
		}
	}
}

func TestProviderRunnerCronFireRejectsNonAllowlistedTool(t *testing.T) {
	registry := tools.NewRegistry()
	tools.RegisterBuiltins(registry)
	registry.Register("echo", tools.Schema{Name: "echo"}, func(_ context.Context, _ tools.Context, args map[string]any) tools.Result {
		return tools.Result{Output: map[string]any{"echo": args["query"]}}
	})
	cfg := config.Default().LLM
	cfg.DefaultModel = "fake/default"
	cfg.Providers["fake"] = config.LLMProviderConfig{
		Type:                 "fake",
		FakeResponseText:     "after {{last_user_message}}",
		FakeToolName:         "echo",
		FakeToolArgsJSON:     `{"query":"blocked"}`,
		FakeFinishReason:     "stop",
		FakeRawFinishReason:  "stop",
		FakePromptTokens:     8,
		FakeCompletionTokens: 8,
		FakeRequestID:        "fake-request-1",
	}
	r := newTestRunner(t, cfg, registry)

	output, err := r.Run(context.Background(), model.InternalEvent{
		EventID:         "evt_cron_forbidden",
		Conversation:    model.Conversation{ConversationID: "conv-cron", ChannelType: "dm", ParticipantID: "u1"},
		SessionKey:      "local:dm:u1",
		ActiveSessionID: "sess_cron_forbidden",
		Payload:         model.EventPayload{Type: "cron_fire", Text: "nightly heartbeat"},
	}, 2, nil)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(output.Trace.ToolExecutions) != 1 {
		t.Fatalf("expected one tool execution, got %+v", output.Trace.ToolExecutions)
	}
	toolExec := output.Trace.ToolExecutions[0]
	if toolExec.Name != "echo" || toolExec.Error == nil || toolExec.Error.Code != model.ErrorCodeForbidden {
		t.Fatalf("expected forbidden echo tool execution, got %+v", output.Trace.ToolExecutions)
	}
	var toolMessage *OutputMessage
	for i := range output.Messages {
		if output.Messages[i].Role == "tool" {
			toolMessage = &output.Messages[i]
			break
		}
	}
	if toolMessage == nil {
		t.Fatalf("expected persisted tool result message, got %+v", output.Messages)
	}
	if toolMessage.ToolName != "echo" || !strings.Contains(toolMessage.Content, model.ErrorCodeForbidden) {
		t.Fatalf("expected forbidden tool result message, got %+v", toolMessage)
	}
	if toolMessage.Visible {
		t.Fatalf("expected forbidden cron tool result to stay hidden, got %+v", toolMessage)
	}
}

func TestProviderRunnerCronFireRejectsWorkspacePatch(t *testing.T) {
	cfg := config.Default().LLM
	cfg.DefaultModel = "fake/default"
	cfg.Providers["fake"] = config.LLMProviderConfig{
		Type:                 "fake",
		FakeResponseText:     "after {{last_user_message}}",
		FakeToolName:         "workspace_patch",
		FakeToolArgsJSON:     `{"path":"IDENTITY.md","old_text":"A","new_text":"B"}`,
		FakeFinishReason:     "stop",
		FakeRawFinishReason:  "stop",
		FakePromptTokens:     8,
		FakeCompletionTokens: 8,
		FakeRequestID:        "fake-request-1",
	}
	r, workspace := newTestRunnerWithWorkspace(t, cfg, nil)
	if err := os.WriteFile(filepath.Join(workspace, "IDENTITY.md"), []byte("A\n"), 0o644); err != nil {
		t.Fatalf("write IDENTITY.md: %v", err)
	}

	output, err := r.Run(context.Background(), model.InternalEvent{
		EventID:         "evt_cron_workspace_patch",
		Conversation:    model.Conversation{ConversationID: "conv-cron", ChannelType: "dm", ParticipantID: "u1"},
		SessionKey:      "local:dm:u1",
		ActiveSessionID: "sess_cron_workspace_patch",
		Payload:         model.EventPayload{Type: "cron_fire", Text: "nightly heartbeat"},
	}, 2, nil)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(output.Trace.ToolExecutions) != 1 {
		t.Fatalf("expected one tool execution, got %+v", output.Trace.ToolExecutions)
	}
	if output.Trace.ToolExecutions[0].Name != "workspace_patch" || output.Trace.ToolExecutions[0].Error == nil || output.Trace.ToolExecutions[0].Error.Code != model.ErrorCodeForbidden {
		t.Fatalf("expected forbidden workspace_patch execution, got %+v", output.Trace.ToolExecutions)
	}
}

func TestCronFireToolPolicyRejectsInjectedPromptFiles(t *testing.T) {
	errBlock := cronFireToolPolicyError(model.ToolCall{
		Name: "context_get",
		Args: map[string]any{"path": "./HEARTBEAT.md"},
	}, map[string]int{})
	if errBlock == nil || errBlock.Code != model.ErrorCodeForbidden {
		t.Fatalf("expected forbidden reread guard, got %+v", errBlock)
	}
	if !strings.Contains(errBlock.Message, "already injected") {
		t.Fatalf("expected injected prompt file hint, got %+v", errBlock)
	}
}

func TestCronFireToolPolicyRejectsBudgetOverflow(t *testing.T) {
	errBlock := cronFireToolPolicyError(model.ToolCall{
		Name: "memory_search",
		Args: map[string]any{"query": "alpha"},
	}, map[string]int{"memory_search": 1})
	if errBlock == nil || errBlock.Code != model.ErrorCodeForbidden {
		t.Fatalf("expected forbidden budget guard, got %+v", errBlock)
	}
	if !strings.Contains(errBlock.Message, "tool budget exhausted") {
		t.Fatalf("expected budget hint, got %+v", errBlock)
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
	statusCount    int
	reasoningCount int
	textCount      int
	starts         []capturedToolStart
	results        []capturedToolResult
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

func (c *captureSink) OnStatus(string, string) { c.statusCount++ }

func (c *captureSink) OnReasoningDelta(string) { c.reasoningCount++ }

func (c *captureSink) OnTextDelta(string) { c.textCount++ }

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

func TestProviderRunnerCronFireDoesNotEmitStreamEvents(t *testing.T) {
	cfg := config.Default().LLM
	cfg.DefaultModel = "fake/default"
	cfg.Providers["fake"] = config.LLMProviderConfig{
		Type:                 "fake",
		FakeResponseText:     "roles={{message_roles}} last={{last_user_message}}",
		FakeToolName:         "memory_search",
		FakeToolArgsJSON:     `{"query":"alpha","visibility":"auto","kind":"any","top_k":1}`,
		FakeFinishReason:     "stop",
		FakeRawFinishReason:  "stop",
		FakePromptTokens:     8,
		FakeCompletionTokens: 8,
		FakeRequestID:        "fake-request-1",
	}
	r, workspace := newTestRunnerWithWorkspace(t, cfg, nil)
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "public"), 0o755); err != nil {
		t.Fatalf("mkdir memory/public: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "memory", "public", "MEMORY.md"), []byte("alpha memory\n"), 0o644); err != nil {
		t.Fatalf("write public memory: %v", err)
	}
	sink := &captureSink{}

	output, err := r.Run(context.Background(), model.InternalEvent{
		EventID:         "evt_cron_stream_hidden",
		Conversation:    model.Conversation{ConversationID: "conv-cron", ChannelType: "dm", ParticipantID: "u1"},
		SessionKey:      "local:dm:u1",
		ActiveSessionID: "sess_cron_stream_hidden",
		Payload:         model.EventPayload{Type: "cron_fire", Text: "nightly heartbeat"},
	}, 2, sink)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if output.AssistantReply != "" || !output.SuppressOutput {
		t.Fatalf("expected suppressed cron output, got %+v", output)
	}
	if sink.statusCount != 0 || sink.reasoningCount != 0 || sink.textCount != 0 || len(sink.starts) != 0 || len(sink.results) != 0 {
		t.Fatalf("expected cron_fire to emit no stream events, got %+v", sink)
	}
}

func containsAll(in string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(in, needle) {
			return false
		}
	}
	return true
}
