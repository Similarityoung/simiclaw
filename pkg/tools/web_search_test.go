package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestWebSearchDuckDuckGoSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "golang tips" {
			t.Fatalf("unexpected query: %q", got)
		}
		if ua := r.Header.Get("User-Agent"); ua == "" {
			t.Fatal("expected user agent header")
		}
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body>
			<a class="result__a" href="https://go.dev/doc/">Go Documentation</a>
			<a class="result__snippet">The Go Programming Language docs.</a>
			<a class="result__a" href="/l/?uddg=https%3A%2F%2Fexample.com%2Fpost%3Fx%3D1%26y%3D2&rut=abc">Example Post</a>
			<a class="result__snippet"> Example   snippet with
			whitespace. </a>
		</body></html>`))
	}))
	defer server.Close()

	reg := NewRegistry()
	RegisterWebSearch(reg, WebSearchOptions{BaseURL: server.URL, MaxResults: 5})
	res := reg.Call(context.Background(), Context{Conversation: model.Conversation{ChannelType: "dm"}}, "web_search", map[string]any{
		"query": "golang tips",
		"top_k": 2,
	})
	if res.Error != nil {
		t.Fatalf("unexpected tool error: %+v", res.Error)
	}

	if provider, _ := res.Output["provider"].(string); provider != "duckduckgo" {
		t.Fatalf("unexpected provider: %q", provider)
	}
	if query, _ := res.Output["query"].(string); query != "golang tips" {
		t.Fatalf("unexpected query: %q", query)
	}

	results, ok := res.Output["results"].([]any)
	if !ok {
		t.Fatalf("results should be []any, got %T", res.Output["results"])
	}
	if len(results) != 2 {
		t.Fatalf("unexpected result length: %d", len(results))
	}
	first := results[0].(map[string]any)
	if first["title"] != "Go Documentation" || first["url"] != "https://go.dev/doc/" {
		t.Fatalf("unexpected first result: %+v", first)
	}
	second := results[1].(map[string]any)
	if second["url"] != "https://example.com/post?x=1&y=2" {
		t.Fatalf("unexpected decoded url: %+v", second)
	}
	if second["snippet"] != "Example snippet with whitespace." {
		t.Fatalf("unexpected normalized snippet: %+v", second)
	}

	summary, _ := res.Output["summary"].(string)
	wantSummary := "The Go Programming Language docs. Example snippet with whitespace."
	if summary != wantSummary {
		t.Fatalf("unexpected summary: %q", summary)
	}
}

func TestWebSearchDuckDuckGoEmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><div class="result--no-result">No results found</div></body></html>`))
	}))
	defer server.Close()

	reg := NewRegistry()
	RegisterWebSearch(reg, WebSearchOptions{BaseURL: server.URL})
	res := reg.Call(context.Background(), Context{}, "web_search", map[string]any{"query": "nothing"})
	if res.Error != nil {
		t.Fatalf("unexpected tool error: %+v", res.Error)
	}
	results, ok := res.Output["results"].([]any)
	if !ok || len(results) != 0 {
		t.Fatalf("unexpected results: %#v", res.Output["results"])
	}
	if summary, _ := res.Output["summary"].(string); summary != "" {
		t.Fatalf("expected empty summary, got %q", summary)
	}
}

func TestWebSearchUsesConfiguredMaxResultsByDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(testDuckDuckGoHTML(6)))
	}))
	defer server.Close()

	reg := NewRegistry()
	RegisterWebSearch(reg, WebSearchOptions{BaseURL: server.URL, MaxResults: 3})
	res := reg.Call(context.Background(), Context{}, "web_search", map[string]any{"query": "default top k"})
	if res.Error != nil {
		t.Fatalf("unexpected tool error: %+v", res.Error)
	}
	results := res.Output["results"].([]any)
	if len(results) != 3 {
		t.Fatalf("unexpected result length: %d", len(results))
	}
}

func TestWebSearchClampsRequestedTopKToConfiguredMax(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(testDuckDuckGoHTML(10)))
	}))
	defer server.Close()

	reg := NewRegistry()
	RegisterWebSearch(reg, WebSearchOptions{BaseURL: server.URL, MaxResults: 3})
	res := reg.Call(context.Background(), Context{}, "web_search", map[string]any{
		"query": "clamp top k",
		"top_k": 99,
	})
	if res.Error != nil {
		t.Fatalf("unexpected tool error: %+v", res.Error)
	}
	results := res.Output["results"].([]any)
	if len(results) != 3 {
		t.Fatalf("unexpected clamped result length: %d", len(results))
	}
}

func TestWebSearchClampsRequestedTopKToGlobalLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(testDuckDuckGoHTML(10)))
	}))
	defer server.Close()

	reg := NewRegistry()
	RegisterWebSearch(reg, WebSearchOptions{BaseURL: server.URL, MaxResults: 99})
	res := reg.Call(context.Background(), Context{}, "web_search", map[string]any{
		"query": "global clamp top k",
		"top_k": 99,
	})
	if res.Error != nil {
		t.Fatalf("unexpected tool error: %+v", res.Error)
	}
	results := res.Output["results"].([]any)
	if len(results) != 8 {
		t.Fatalf("unexpected clamped result length: %d", len(results))
	}
}

func TestWebSearchRejectsUnexpectedResponseFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body>weird</body></html>`))
	}))
	defer server.Close()

	reg := NewRegistry()
	RegisterWebSearch(reg, WebSearchOptions{BaseURL: server.URL})
	res := reg.Call(context.Background(), Context{}, "web_search", map[string]any{"query": "broken"})
	if res.Error == nil || res.Error.Code != model.ErrorCodeInternal {
		t.Fatalf("expected internal error, got %+v", res.Error)
	}
}

func TestWebSearchRejectsNon200Response(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer server.Close()

	reg := NewRegistry()
	RegisterWebSearch(reg, WebSearchOptions{BaseURL: server.URL})
	res := reg.Call(context.Background(), Context{}, "web_search", map[string]any{"query": "status"})
	if res.Error == nil || res.Error.Code != model.ErrorCodeInternal {
		t.Fatalf("expected internal error, got %+v", res.Error)
	}
}

func TestWebSearchRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><body>" + strings.Repeat("x", webSearchBodyLimitBytes+1) + "</body></html>"))
	}))
	defer server.Close()

	reg := NewRegistry()
	RegisterWebSearch(reg, WebSearchOptions{BaseURL: server.URL})
	res := reg.Call(context.Background(), Context{}, "web_search", map[string]any{"query": "large"})
	if res.Error == nil || res.Error.Code != model.ErrorCodeInternal {
		t.Fatalf("expected internal error, got %+v", res.Error)
	}
}

func TestWebSearchReturnsCanceledWhenContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(testDuckDuckGoHTML(1)))
	}))
	defer server.Close()

	reg := NewRegistry()
	RegisterWebSearch(reg, WebSearchOptions{BaseURL: server.URL})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res := reg.Call(ctx, Context{}, "web_search", map[string]any{"query": "cancel"})
	if res.Error == nil || res.Error.Code != model.ErrorCodeCanceled {
		t.Fatalf("expected canceled error, got %+v", res.Error)
	}
}

func TestWebSearchTreatsClientTimeoutAsInternal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte(testDuckDuckGoHTML(1)))
	}))
	defer server.Close()

	reg := NewRegistry()
	RegisterWebSearch(reg, WebSearchOptions{
		BaseURL: server.URL,
		Client:  &http.Client{Timeout: 10 * time.Millisecond},
	})
	res := reg.Call(context.Background(), Context{}, "web_search", map[string]any{"query": "timeout"})
	if res.Error == nil || res.Error.Code != model.ErrorCodeInternal {
		t.Fatalf("expected internal error, got %+v", res.Error)
	}
}

func TestSummarizeWebSearchResultsUsesFirstThreeNonEmptySnippets(t *testing.T) {
	got := summarizeWebSearchResults([]webSearchResult{
		{Snippet: "  alpha  beta  "},
		{Snippet: ""},
		{Snippet: "gamma\n\tdelta"},
		{Snippet: "epsilon"},
		{Snippet: "zeta"},
	})
	if got != "alpha beta gamma delta epsilon" {
		t.Fatalf("unexpected summary: %q", got)
	}
}

func TestSummarizeWebSearchResultsTruncatesToFixedRunes(t *testing.T) {
	got := summarizeWebSearchResults([]webSearchResult{
		{Snippet: strings.Repeat("界", webSearchSummaryMaxRunes+10)},
	})
	if len([]rune(got)) != webSearchSummaryMaxRunes {
		t.Fatalf("unexpected rune length: %d", len([]rune(got)))
	}
}

func TestSummarizeWebSearchResultsReturnsEmptyWhenNoSnippets(t *testing.T) {
	got := summarizeWebSearchResults([]webSearchResult{{Snippet: ""}, {Snippet: "   "}})
	if got != "" {
		t.Fatalf("expected empty summary, got %q", got)
	}
}

func testDuckDuckGoHTML(count int) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><body>")
	for i := 0; i < count; i++ {
		b.WriteString(fmt.Sprintf(`<a class="result__a" href="https://example.com/%d">Result %d</a>`, i+1, i+1))
		b.WriteString(fmt.Sprintf(`<a class="result__snippet">Snippet %d</a>`, i+1))
	}
	b.WriteString("</body></html>")
	return b.String()
}
