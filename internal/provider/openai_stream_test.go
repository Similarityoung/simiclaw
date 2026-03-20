package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	openai "github.com/openai/openai-go/v3"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/testutil/logcapture"
	"github.com/similarityyoung/simiclaw/pkg/logging"
)

func TestToolCallAccumulatorBindsLateToolCallID(t *testing.T) {
	acc := newToolCallAccumulator()
	acc.AddChunk(mustChunk(t, `{"id":"chatcmpl_1","model":"test","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":\"par"}}]}}]}`))
	acc.AddChunk(mustChunk(t, `{"id":"chatcmpl_1","model":"test","choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_1","index":0,"type":"function","function":{"name":"search","arguments":"is\"}"}}],"content":""},"finish_reason":"tool_calls"}]}`))

	toolCalls, err := acc.BuildToolCalls(0)
	if err != nil {
		t.Fatalf("BuildToolCalls returned error: %v", err)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].ToolCallID != "call_1" {
		t.Fatalf("expected tool_call_id=call_1, got %+v", toolCalls[0])
	}
	if toolCalls[0].Name != "search" {
		t.Fatalf("expected tool name search, got %+v", toolCalls[0])
	}
	if got := toolCalls[0].Args["q"]; got != "paris" {
		t.Fatalf("expected parsed args q=paris, got %+v", toolCalls[0].Args)
	}
}

func TestToolCallAccumulatorDoesNotCrossChoices(t *testing.T) {
	acc := newToolCallAccumulator()
	acc.AddChunk(mustChunk(t, `{"id":"chatcmpl_2","model":"test","choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_a","index":0,"type":"function","function":{"name":"lookup","arguments":"{\"city\":\"beijing\"}"}}]}},{"index":1,"delta":{"tool_calls":[{"id":"call_b","index":0,"type":"function","function":{"name":"lookup","arguments":"{\"city\":\"shanghai\"}"}}]}}]}`))

	choice0, err := acc.BuildToolCalls(0)
	if err != nil {
		t.Fatalf("choice0 BuildToolCalls error: %v", err)
	}
	choice1, err := acc.BuildToolCalls(1)
	if err != nil {
		t.Fatalf("choice1 BuildToolCalls error: %v", err)
	}
	if len(choice0) != 1 || len(choice1) != 1 {
		t.Fatalf("unexpected tool call counts: choice0=%d choice1=%d", len(choice0), len(choice1))
	}
	if choice0[0].Args["city"] != "beijing" || choice1[0].Args["city"] != "shanghai" {
		t.Fatalf("tool args crossed choices: choice0=%+v choice1=%+v", choice0[0].Args, choice1[0].Args)
	}
}

func TestToolCallAccumulatorRejectsInvalidParallelArguments(t *testing.T) {
	acc := newToolCallAccumulator()
	acc.AddChunk(mustChunk(t, `{"id":"chatcmpl_3","model":"test","choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_a","index":0,"type":"function","function":{"name":"lookup","arguments":"{\"city\":\"beijing\"}"}},{"id":"call_b","index":1,"type":"function","function":{"name":"lookup","arguments":"{\"city\":\"shanghai\""}}]}}]}`))

	if _, err := acc.BuildToolCalls(0); err == nil {
		t.Fatalf("expected invalid JSON error")
	}
}

func TestOpenAICompatibleProviderStreamFailureDoesNotEchoPromptBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl_bad\",\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"id\":\"call_1\",\"index\":0,\"type\":\"function\",\"function\":{\"name\":\"search\",\"arguments\":\"{not-json\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer server.Close()

	p, err := newOpenAICompatibleProvider("openai", config.LLMProviderConfig{
		Type:    "openai_compatible",
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: config.Duration{Duration: 200 * time.Millisecond},
	})
	if err != nil {
		t.Fatalf("newOpenAICompatibleProvider returned error: %v", err)
	}

	prompt := "stream prompt body"
	var out string
	out = logcapture.CaptureStdout(t, func() {
		if err := logging.Init("debug"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		_, err = p.StreamChat(context.Background(), ChatRequest{
			Model: "test-model",
			Messages: []ChatMessage{{
				Role:    "user",
				Content: prompt,
			}},
		}, nil)
		_ = logging.Sync()
	})
	if err == nil {
		t.Fatal("expected StreamChat error")
	}
	if !strings.Contains(err.Error(), `invalid tool arguments for "search"`) {
		t.Fatalf("unexpected stream error: %v", err)
	}
	if strings.Contains(err.Error(), prompt) {
		t.Fatalf("stream error leaked prompt body: %v", err)
	}
	if strings.Contains(out, prompt) {
		t.Fatalf("stream debug log leaked prompt body: %q", out)
	}
	if !strings.Contains(out, "[provider.openai] stream accumulator failed") {
		t.Fatalf("expected provider debug log, got %q", out)
	}
}

func mustChunk(t *testing.T, raw string) openai.ChatCompletionChunk {
	t.Helper()
	var chunk openai.ChatCompletionChunk
	if err := chunk.UnmarshalJSON([]byte(raw)); err != nil {
		t.Fatalf("unmarshal chunk: %v", err)
	}
	return chunk
}
