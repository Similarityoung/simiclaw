package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestWebFetchSuccessHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/post" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
		<html>
			<head>
				<title>Example Article</title>
				<style>.hidden{display:none}</style>
			</head>
			<body>
				<article>
					<h1>Hello</h1>
					<p>world &amp; beyond.</p>
					<script>console.log("ignore")</script>
				</article>
			</body>
		</html>`))
	}))
	defer server.Close()

	reg := NewRegistry()
	RegisterWebFetch(reg, WebFetchOptions{AllowPrivateHosts: true})
	res := reg.Call(context.Background(), Context{}, "web_fetch", map[string]any{
		"url": server.URL + "/post",
	})
	if res.Error != nil {
		t.Fatalf("unexpected tool error: %+v", res.Error)
	}

	if got := res.Output["title"]; got != "Example Article" {
		t.Fatalf("unexpected title: %#v", got)
	}
	if got := res.Output["content_type"]; got != "text/html" {
		t.Fatalf("unexpected content type: %#v", got)
	}
	if got := res.Output["status_code"]; got != http.StatusOK {
		t.Fatalf("unexpected status code: %#v", got)
	}
	if got := res.Output["cached"]; got != false {
		t.Fatalf("expected uncached result, got %#v", got)
	}
	text, _ := res.Output["text"].(string)
	if !strings.Contains(text, "Hello world & beyond.") {
		t.Fatalf("unexpected text: %q", text)
	}
	if strings.Contains(text, "console.log") {
		t.Fatalf("expected scripts to be removed, got %q", text)
	}
	if truncated, _ := res.Output["truncated"].(bool); truncated {
		t.Fatalf("expected non-truncated result, got %+v", res.Output)
	}
}

func TestWebFetchUsesCacheWithinTTL(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("cached text"))
	}))
	defer server.Close()

	reg := NewRegistry()
	RegisterWebFetch(reg, WebFetchOptions{
		AllowPrivateHosts: true,
		CacheTTL:          time.Minute,
	})

	first := reg.Call(context.Background(), Context{}, "web_fetch", map[string]any{"url": server.URL + "/memo"})
	if first.Error != nil {
		t.Fatalf("unexpected first error: %+v", first.Error)
	}
	second := reg.Call(context.Background(), Context{}, "web_fetch", map[string]any{"url": server.URL + "/memo"})
	if second.Error != nil {
		t.Fatalf("unexpected second error: %+v", second.Error)
	}
	if calls != 1 {
		t.Fatalf("expected single upstream call due to cache, got %d", calls)
	}
	if second.Output["cached"] != true {
		t.Fatalf("expected cached second result, got %#v", second.Output["cached"])
	}
}

func TestWebFetchRejectsMissingURL(t *testing.T) {
	reg := NewRegistry()
	RegisterWebFetch(reg, WebFetchOptions{})

	res := reg.Call(context.Background(), Context{}, "web_fetch", map[string]any{})
	if res.Error == nil || res.Error.Code != model.ErrorCodeInvalidArgument {
		t.Fatalf("expected invalid argument error, got %+v", res.Error)
	}
}

func TestWebFetchRejectsPrivateHostsByDefault(t *testing.T) {
	reg := NewRegistry()
	RegisterWebFetch(reg, WebFetchOptions{})

	res := reg.Call(context.Background(), Context{}, "web_fetch", map[string]any{
		"url": "http://127.0.0.1/private",
	})
	if res.Error == nil || res.Error.Code != model.ErrorCodeForbidden {
		t.Fatalf("expected forbidden error, got %+v", res.Error)
	}
}

func TestWebFetchRejectsUnsupportedContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("not really png"))
	}))
	defer server.Close()

	reg := NewRegistry()
	RegisterWebFetch(reg, WebFetchOptions{AllowPrivateHosts: true})
	res := reg.Call(context.Background(), Context{}, "web_fetch", map[string]any{
		"url": server.URL + "/image",
	})
	if res.Error == nil || res.Error.Code != model.ErrorCodeInvalidArgument {
		t.Fatalf("expected invalid argument error, got %+v", res.Error)
	}
}

func TestWebFetchTruncatesToRequestedMaxChars(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(strings.Repeat("界", 400)))
	}))
	defer server.Close()

	reg := NewRegistry()
	RegisterWebFetch(reg, WebFetchOptions{AllowPrivateHosts: true})
	res := reg.Call(context.Background(), Context{}, "web_fetch", map[string]any{
		"url":       server.URL + "/long",
		"max_chars": 200,
	})
	if res.Error != nil {
		t.Fatalf("unexpected tool error: %+v", res.Error)
	}

	text, _ := res.Output["text"].(string)
	if len([]rune(text)) != 200 {
		t.Fatalf("unexpected text length: %d", len([]rune(text)))
	}
	if res.Output["truncated"] != true {
		t.Fatalf("expected truncated result, got %#v", res.Output["truncated"])
	}
}
