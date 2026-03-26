package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	"github.com/similarityyoung/simiclaw/internal/testutil/logcapture"
	"github.com/similarityyoung/simiclaw/pkg/logging"
)

func TestOpenAICompatibleProviderStreamChatUsesRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	p, err := newOpenAICompatibleProvider("openai", config.LLMProviderConfig{
		Type:    "openai_compatible",
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: config.Duration{Duration: 50 * time.Millisecond},
	})
	if err != nil {
		t.Fatalf("newOpenAICompatibleProvider returned error: %v", err)
	}

	start := time.Now()
	_, err = p.StreamChat(context.Background(), ChatRequest{
		Model: "test-model",
		Messages: []ChatMessage{{
			Role:    "user",
			Content: "hello",
		}},
	}, nil)
	if err == nil {
		t.Fatal("expected StreamChat timeout error")
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("expected StreamChat to timeout quickly, took %s", elapsed)
	}
	if kind := kernel.CapabilityErrorKindOf(err); kind != kernel.CapabilityErrorTimeout {
		t.Fatalf("CapabilityErrorKindOf() = %q want %q", kind, kernel.CapabilityErrorTimeout)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded cause, got %v", err)
	}
}

func TestOpenAICompatibleProviderStreamChatSucceedsWithinRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		time.Sleep(20 * time.Millisecond)
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl_1\",\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl_1\",\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n")
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := p.StreamChat(ctx, ChatRequest{
		Model: "test-model",
		Messages: []ChatMessage{{
			Role:    "user",
			Content: "hello",
		}},
	}, nil)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if result.Text != "hello" {
		t.Fatalf("unexpected stream result: %+v", result)
	}
}

func TestOpenAICompatibleProviderChatStillUsesRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(80 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"chatcmpl_2","model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()

	p, err := newOpenAICompatibleProvider("openai", config.LLMProviderConfig{
		Type:    "openai_compatible",
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: config.Duration{Duration: 50 * time.Millisecond},
	})
	if err != nil {
		t.Fatalf("newOpenAICompatibleProvider returned error: %v", err)
	}

	start := time.Now()
	_, err = p.Chat(context.Background(), ChatRequest{
		Model: "test-model",
		Messages: []ChatMessage{{
			Role:    "user",
			Content: "hello",
		}},
	})
	if err == nil {
		t.Fatal("expected Chat timeout error")
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("expected Chat to timeout quickly, took %s", elapsed)
	}
	if kind := kernel.CapabilityErrorKindOf(err); kind != kernel.CapabilityErrorTimeout {
		t.Fatalf("CapabilityErrorKindOf() = %q want %q", kind, kernel.CapabilityErrorTimeout)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded cause, got %v", err)
	}
}

func TestOpenAICompatibleProviderErrorsDoNotEchoPromptBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(80 * time.Millisecond)
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = fmt.Fprint(w, `{"error":{"message":"upstream timeout"}}`)
	}))
	defer server.Close()

	p, err := newOpenAICompatibleProvider("openai", config.LLMProviderConfig{
		Type:    "openai_compatible",
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: config.Duration{Duration: 30 * time.Millisecond},
	})
	if err != nil {
		t.Fatalf("newOpenAICompatibleProvider returned error: %v", err)
	}

	prompt := "super secret prompt body"
	var out string
	out = logcapture.CaptureStdout(t, func() {
		if err := logging.Init("debug"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		_, err = p.Chat(context.Background(), ChatRequest{
			Model: "test-model",
			Messages: []ChatMessage{{
				Role:    "user",
				Content: prompt,
			}},
		})
		_ = logging.Sync()
	})
	if err == nil {
		t.Fatal("expected Chat timeout error")
	}
	if strings.Contains(err.Error(), prompt) {
		t.Fatalf("error leaked prompt body: %v", err)
	}
	if strings.Contains(out, prompt) {
		t.Fatalf("debug log leaked prompt body: %q", out)
	}
	if !strings.Contains(out, "[provider.openai] chat transport failed") {
		t.Fatalf("expected provider debug log, got %q", out)
	}
}
