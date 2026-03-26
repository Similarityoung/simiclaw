package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
)

type stubStreamSink struct {
	chunks []string
}

func (s *stubStreamSink) OnReasoningDelta(string) {}

func (s *stubStreamSink) OnTextDelta(delta string) {
	s.chunks = append(s.chunks, delta)
}

func TestFakeProviderStreamChatEmitsChunksAndFinalText(t *testing.T) {
	provider := &fakeProvider{name: "fake", cfg: config.LLMProviderConfig{Type: "fake", FakeResponseText: strings.Repeat("已收到: {{last_user_message}}", 2), FakeFinishReason: "stop", FakeRawFinishReason: "stop", FakePromptTokens: 8, FakeCompletionTokens: 8, FakeRequestID: "fake-req"}}
	sink := &stubStreamSink{}
	res, err := provider.StreamChat(context.Background(), ChatRequest{
		Model:    "default",
		Messages: []ChatMessage{{Role: "user", Content: "hello world"}},
	}, sink)
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	if got := strings.Join(sink.chunks, ""); got != res.Text {
		t.Fatalf("joined chunks = %q want %q", got, res.Text)
	}
	if len(sink.chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(sink.chunks))
	}
}

func TestFakeProviderStreamChatSkipsDeltaWhenReturningToolCall(t *testing.T) {
	provider := &fakeProvider{name: "fake", cfg: config.LLMProviderConfig{Type: "fake", FakeResponseText: "ignored", FakeToolName: "echo", FakeFinishReason: "tool_calls", FakeRawFinishReason: "tool_calls", FakePromptTokens: 8, FakeCompletionTokens: 8, FakeRequestID: "fake-req"}}
	sink := &stubStreamSink{}
	res, err := provider.StreamChat(context.Background(), ChatRequest{
		Model:    "default",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	}, sink)
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	if len(sink.chunks) != 0 {
		t.Fatal("expected no delta when provider returns tool calls")
	}
	if len(res.ToolCalls) != 1 {
		t.Fatalf("tool calls len = %d want 1", len(res.ToolCalls))
	}
}

func TestFactoryResolveReturnsCapabilityInvoker(t *testing.T) {
	cfg := config.Default().LLM
	cfg.DefaultModel = "fake/default"
	cfg.Providers["fake"] = config.LLMProviderConfig{
		Type:                "fake",
		FakeResponseText:    "已收到: {{last_user_message}}",
		FakeFinishReason:    "stop",
		FakeRawFinishReason: "stop",
	}
	factory, err := NewFactory(cfg)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}

	invoker, actualModel, err := factory.Resolve("fake/capability-model")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if actualModel != "capability-model" {
		t.Fatalf("actualModel = %q want capability-model", actualModel)
	}

	res, err := invoker.StreamChat(context.Background(), kernel.ModelRequest{
		Model:    actualModel,
		Messages: []kernel.ModelMessage{{Role: "user", Content: "hello"}},
	}, nil)
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	if res.Provider != "fake" {
		t.Fatalf("provider = %q want fake", res.Provider)
	}
}
