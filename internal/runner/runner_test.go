package runner

import (
	"context"
	"testing"

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

func newTestRunner(t *testing.T, llm config.LLMConfig, registry *tools.Registry) *ProviderRunner {
	t.Helper()
	workspace := t.TempDir()
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
