package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/config"
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
}
