package tools

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

const (
	defaultWebSearchTimeout    = 10 * time.Second
	defaultWebSearchMaxResults = 5
	minWebSearchTopK           = 1
	maxWebSearchTopK           = 8
	webSearchSummarySources    = 3
	webSearchSummaryMaxRunes   = 600
	webSearchBodyLimitBytes    = 1 << 20
	defaultDuckDuckGoBaseURL   = "https://html.duckduckgo.com/html/"
	webSearchUserAgent         = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
)

var (
	reDuckDuckGoResultLink = regexp.MustCompile(`<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>([\s\S]*?)</a>`)
	reDuckDuckGoSnippet    = regexp.MustCompile(`<a[^>]*class="[^"]*result__snippet[^"]*"[^>]*>([\s\S]*?)</a>`)
	reHTMLTags             = regexp.MustCompile(`<[^>]+>`)
	reWhitespace           = regexp.MustCompile(`\s+`)
)

type WebSearchOptions struct {
	Timeout    time.Duration
	MaxResults int
	BaseURL    string
	Client     *http.Client
}

type webSearchTool struct {
	baseURL    string
	client     *http.Client
	maxResults int
}

type webSearchResult struct {
	Title   string
	URL     string
	Snippet string
}

func RegisterWebSearch(reg *Registry, opts WebSearchOptions) {
	minimum := 1.0
	maximum := 8.0
	schema := Schema{
		Name:        "web_search",
		Description: "查询当前外部公开网页信息，返回标题、URL 和摘要片段。",
		Parameters: ParameterSchema{
			Type: "object",
			Properties: map[string]ParameterSchema{
				"query": {
					Type:        "string",
					Description: "搜索关键词或问题，适用于需要最新公开信息的场景。",
				},
				"top_k": {
					Type:        "integer",
					Description: "返回结果条数，范围 1 到 8，默认使用工具配置。",
					Minimum:     &minimum,
					Maximum:     &maximum,
				},
			},
			Required: []string{"query"},
		},
	}
	tool := newWebSearchTool(opts)
	reg.Register("web_search", schema, tool.handle)
}

func newWebSearchTool(opts WebSearchOptions) *webSearchTool {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultWebSearchTimeout
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	baseURL := strings.TrimSpace(opts.BaseURL)
	if baseURL == "" {
		baseURL = defaultDuckDuckGoBaseURL
	}
	return &webSearchTool{
		baseURL:    baseURL,
		client:     client,
		maxResults: normalizeWebSearchMaxResults(opts.MaxResults),
	}
}

func (t *webSearchTool) handle(ctx context.Context, _ Context, args map[string]any) Result {
	query := strings.TrimSpace(stringArg(args["query"]))
	if query == "" {
		return Result{Error: &model.ErrorBlock{Code: model.ErrorCodeInvalidArgument, Message: "web_search failed: query is required"}}
	}

	topK := resolveWebSearchTopK(args["top_k"], t.maxResults)
	results, err := t.search(ctx, query, topK)
	if err != nil {
		if ctx.Err() != nil {
			return Result{Error: &model.ErrorBlock{Code: model.ErrorCodeCanceled, Message: fmt.Sprintf("web_search canceled: %v", ctx.Err())}}
		}
		return Result{Error: &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: fmt.Sprintf("web_search failed: %v", err)}}
	}

	outputResults := make([]any, 0, len(results))
	for _, item := range results {
		outputResults = append(outputResults, map[string]any{
			"title":   item.Title,
			"url":     item.URL,
			"snippet": item.Snippet,
		})
	}

	return Result{Output: map[string]any{
		"provider": "duckduckgo",
		"query":    query,
		"summary":  summarizeWebSearchResults(results),
		"results":  outputResults,
	}}
}

func (t *webSearchTool) search(ctx context.Context, query string, topK int) ([]webSearchResult, error) {
	searchURL, err := buildDuckDuckGoSearchURL(t.baseURL, query)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", webSearchUserAgent)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := readLimitedBody(resp.Body, webSearchBodyLimitBytes)
	if err != nil {
		return nil, err
	}

	return extractDuckDuckGoResults(string(body), topK)
}

func buildDuckDuckGoSearchURL(baseURL, query string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("invalid web search base url: %w", err)
	}
	values := parsed.Query()
	values.Set("q", query)
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func readLimitedBody(r io.Reader, limit int64) ([]byte, error) {
	reader := &io.LimitedReader{R: r, N: limit + 1}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("response body exceeds 1 MiB limit")
	}
	return body, nil
}

func extractDuckDuckGoResults(body string, topK int) ([]webSearchResult, error) {
	lowerBody := strings.ToLower(body)
	if !strings.Contains(lowerBody, "<html") {
		return nil, errors.New("non-HTML response body")
	}

	linkMatches := reDuckDuckGoResultLink.FindAllStringSubmatch(body, -1)
	if len(linkMatches) == 0 {
		if hasDuckDuckGoNoResults(lowerBody) {
			return []webSearchResult{}, nil
		}
		return nil, errors.New("unexpected response format")
	}

	snippetMatches := reDuckDuckGoSnippet.FindAllStringSubmatch(body, len(linkMatches))
	results := make([]webSearchResult, 0, minInt(len(linkMatches), topK))
	for idx, match := range linkMatches {
		if len(results) >= topK {
			break
		}
		if len(match) < 3 {
			continue
		}
		link := decodeDuckDuckGoURL(match[1])
		title := cleanHTMLText(match[2])
		if title == "" || link == "" {
			continue
		}
		snippet := ""
		if idx < len(snippetMatches) && len(snippetMatches[idx]) >= 2 {
			snippet = cleanHTMLText(snippetMatches[idx][1])
		}
		results = append(results, webSearchResult{
			Title:   title,
			URL:     link,
			Snippet: snippet,
		})
	}

	if len(results) == 0 {
		if hasDuckDuckGoNoResults(lowerBody) {
			return []webSearchResult{}, nil
		}
		return nil, errors.New("unexpected response format")
	}
	return results, nil
}

func hasDuckDuckGoNoResults(lowerBody string) bool {
	return strings.Contains(lowerBody, "result--no-result") ||
		strings.Contains(lowerBody, "no results found") ||
		strings.Contains(lowerBody, "did not match any documents")
}

func decodeDuckDuckGoURL(raw string) string {
	trimmed := html.UnescapeString(strings.TrimSpace(raw))
	if strings.Contains(trimmed, "uddg=") {
		parsed, err := url.Parse(trimmed)
		if err == nil {
			if target := parsed.Query().Get("uddg"); target != "" {
				if decoded, decodeErr := url.QueryUnescape(target); decodeErr == nil {
					return decoded
				}
				return target
			}
		}
	}
	return trimmed
}

func cleanHTMLText(raw string) string {
	plain := reHTMLTags.ReplaceAllString(raw, " ")
	plain = html.UnescapeString(plain)
	return normalizeWhitespace(plain)
}

func summarizeWebSearchResults(results []webSearchResult) string {
	if len(results) == 0 {
		return ""
	}
	snippets := make([]string, 0, webSearchSummarySources)
	for _, item := range results {
		snippet := normalizeWhitespace(item.Snippet)
		if snippet == "" {
			continue
		}
		snippets = append(snippets, snippet)
		if len(snippets) == webSearchSummarySources {
			break
		}
	}
	if len(snippets) == 0 {
		return ""
	}
	return truncateRunes(strings.Join(snippets, " "), webSearchSummaryMaxRunes)
}

func normalizeWhitespace(s string) string {
	return strings.TrimSpace(reWhitespace.ReplaceAllString(s, " "))
}

func truncateRunes(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}

func stringArg(v any) string {
	s, _ := v.(string)
	return s
}

func normalizeWebSearchMaxResults(v int) int {
	switch {
	case v <= 0:
		return defaultWebSearchMaxResults
	case v > maxWebSearchTopK:
		return maxWebSearchTopK
	default:
		return v
	}
}

func resolveWebSearchTopK(raw any, fallback int) int {
	maxAllowed := normalizeWebSearchMaxResults(fallback)
	switch value := raw.(type) {
	case int:
		return minInt(clampRequestedWebSearchTopK(value), maxAllowed)
	case int32:
		return minInt(clampRequestedWebSearchTopK(int(value)), maxAllowed)
	case int64:
		return minInt(clampRequestedWebSearchTopK(int(value)), maxAllowed)
	case float64:
		return minInt(clampRequestedWebSearchTopK(int(value)), maxAllowed)
	default:
		return maxAllowed
	}
}

func clampRequestedWebSearchTopK(v int) int {
	switch {
	case v < minWebSearchTopK:
		return minWebSearchTopK
	case v > maxWebSearchTopK:
		return maxWebSearchTopK
	default:
		return v
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
