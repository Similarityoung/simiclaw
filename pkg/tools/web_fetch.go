package tools

import (
	"context"
	"errors"
	"fmt"
	"html"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

const (
	defaultWebFetchTimeout       = 12 * time.Second
	defaultWebFetchCacheTTL      = 2 * time.Minute
	defaultWebFetchMaxChars      = 8000
	minWebFetchMaxChars          = 200
	maxWebFetchMaxChars          = 20000
	defaultWebFetchBodyLimit     = 1 << 20
	defaultWebFetchCacheCapacity = 128
	maxWebFetchRedirects         = 5
)

var (
	reHTMLTitle      = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reHTMLDropBlocks = regexp.MustCompile(`(?is)<(?:script|style|noscript|svg)[^>]*>.*?</(?:script|style|noscript|svg)>`)
	reHTMLLineBreaks = regexp.MustCompile(`(?i)<\s*(br|p|div|section|article|li|tr|td|th|h[1-6]|pre|blockquote)[^>]*>`)
	reHTMLBlockEnds  = regexp.MustCompile(`(?i)</\s*(p|div|section|article|li|tr|td|th|h[1-6]|pre|blockquote)\s*>`)
)

type WebFetchOptions struct {
	Timeout           time.Duration
	CacheTTL          time.Duration
	MaxChars          int
	BodyLimitBytes    int64
	CacheCapacity     int
	AllowPrivateHosts bool
	Client            *http.Client
}

type webFetchTool struct {
	service *webFetchService
}

type webFetchService struct {
	repo     webFetchRepository
	maxChars int
}

type webFetchRepository interface {
	Fetch(ctx context.Context, targetURL string) (webFetchFetchResult, error)
}

type webFetchFetchResult struct {
	Document webFetchDocument
	Cached   bool
}

type webFetchDocument struct {
	URL         string
	FinalURL    string
	StatusCode  int
	ContentType string
	Title       string
	Text        string
}

type webFetchCacheEntry struct {
	Result    webFetchFetchResult
	ExpiresAt time.Time
	LastUsed  time.Time
}

type cachedWebFetchRepository struct {
	base     webFetchRepository
	ttl      time.Duration
	capacity int
	mu       sync.Mutex
	entries  map[string]webFetchCacheEntry
}

type httpWebFetchRepository struct {
	client            *http.Client
	bodyLimitBytes    int64
	allowPrivateHosts bool
}

type webFetchError struct {
	Code    string
	Message string
}

func (e *webFetchError) Error() string {
	return e.Message
}

func RegisterWebFetch(reg *Registry, opts WebFetchOptions) {
	minimum := float64(minWebFetchMaxChars)
	maximum := float64(maxWebFetchMaxChars)
	schema := Schema{
		Name:        "web_fetch",
		Description: "抓取单个公开网页 URL 的正文内容，返回标题、规范化文本和响应元数据。",
		Parameters: ParameterSchema{
			Type: "object",
			Properties: map[string]ParameterSchema{
				"url": {
					Type:        "string",
					Description: "要抓取的公开 http/https URL。",
				},
				"max_chars": {
					Type:        "integer",
					Description: "返回正文最大字符数，范围 200 到 20000，默认使用工具配置。",
					Minimum:     &minimum,
					Maximum:     &maximum,
				},
			},
			Required: []string{"url"},
		},
	}
	reg.Register("web_fetch", schema, newWebFetchTool(opts).handle)
}

func newWebFetchTool(opts WebFetchOptions) *webFetchTool {
	service := newWebFetchService(newCachedWebFetchRepository(newHTTPWebFetchRepository(opts), opts), opts)
	return &webFetchTool{service: service}
}

func newWebFetchService(repo webFetchRepository, opts WebFetchOptions) *webFetchService {
	return &webFetchService{
		repo:     repo,
		maxChars: normalizeWebFetchMaxChars(opts.MaxChars),
	}
}

func newCachedWebFetchRepository(base webFetchRepository, opts WebFetchOptions) webFetchRepository {
	ttl := opts.CacheTTL
	if ttl <= 0 {
		ttl = defaultWebFetchCacheTTL
	}
	return &cachedWebFetchRepository{
		base:     base,
		ttl:      ttl,
		capacity: normalizeWebFetchCacheCapacity(opts.CacheCapacity),
		entries:  map[string]webFetchCacheEntry{},
	}
}

func newHTTPWebFetchRepository(opts WebFetchOptions) webFetchRepository {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultWebFetchTimeout
	}
	client := opts.Client
	if client == nil {
		client = newWebFetchHTTPClient(timeout, opts.AllowPrivateHosts)
	}
	return &httpWebFetchRepository{
		client:            client,
		bodyLimitBytes:    normalizeWebFetchBodyLimit(opts.BodyLimitBytes),
		allowPrivateHosts: opts.AllowPrivateHosts,
	}
}

func newWebFetchHTTPClient(timeout time.Duration, allowPrivateHosts bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: timeout}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}
		if err := validateWebFetchHost(ctx, host, allowPrivateHosts); err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, address)
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxWebFetchRedirects {
			return errors.New("too many redirects")
		}
		return validateWebFetchTarget(req.Context(), req.URL, allowPrivateHosts)
	}
	return client
}

func (t *webFetchTool) handle(ctx context.Context, _ Context, args map[string]any) Result {
	rawURL := strings.TrimSpace(stringArg(args["url"]))
	if rawURL == "" {
		return Result{Error: &model.ErrorBlock{Code: model.ErrorCodeInvalidArgument, Message: "web_fetch failed: url is required"}}
	}

	normalizedURL, err := normalizeWebFetchURL(rawURL)
	if err != nil {
		return Result{Error: &model.ErrorBlock{Code: model.ErrorCodeInvalidArgument, Message: fmt.Sprintf("web_fetch failed: %v", err)}}
	}

	maxChars := resolveWebFetchMaxChars(args["max_chars"], t.service.maxChars)
	result, err := t.service.fetch(ctx, normalizedURL, maxChars)
	if err != nil {
		if ctx.Err() != nil {
			return Result{Error: &model.ErrorBlock{Code: model.ErrorCodeCanceled, Message: fmt.Sprintf("web_fetch canceled: %v", ctx.Err())}}
		}
		var fetchErr *webFetchError
		if errors.As(err, &fetchErr) {
			return Result{Error: &model.ErrorBlock{Code: fetchErr.Code, Message: "web_fetch failed: " + fetchErr.Message}}
		}
		return Result{Error: &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: fmt.Sprintf("web_fetch failed: %v", err)}}
	}

	return Result{Output: map[string]any{
		"url":          result.Document.URL,
		"final_url":    result.Document.FinalURL,
		"status_code":  result.Document.StatusCode,
		"content_type": result.Document.ContentType,
		"title":        result.Document.Title,
		"text":         result.Document.Text,
		"total_chars":  result.TotalChars,
		"truncated":    result.Truncated,
		"cached":       result.Cached,
	}}
}

type webFetchResponse struct {
	Document   webFetchDocument
	Cached     bool
	TotalChars int
	Truncated  bool
}

func (s *webFetchService) fetch(ctx context.Context, targetURL string, maxChars int) (webFetchResponse, error) {
	result, err := s.repo.Fetch(ctx, targetURL)
	if err != nil {
		return webFetchResponse{}, err
	}

	textRunes := []rune(result.Document.Text)
	totalChars := len(textRunes)
	truncated := false
	if len(textRunes) > maxChars {
		truncated = true
		result.Document.Text = string(textRunes[:maxChars])
	}

	return webFetchResponse{
		Document:   result.Document,
		Cached:     result.Cached,
		TotalChars: totalChars,
		Truncated:  truncated,
	}, nil
}

func (r *cachedWebFetchRepository) Fetch(ctx context.Context, targetURL string) (webFetchFetchResult, error) {
	now := time.Now().UTC()

	r.mu.Lock()
	entry, ok := r.entries[targetURL]
	if ok && now.Before(entry.ExpiresAt) {
		entry.LastUsed = now
		r.entries[targetURL] = entry
		r.mu.Unlock()
		return webFetchFetchResult{
			Document: entry.Result.Document,
			Cached:   true,
		}, nil
	}
	if ok {
		delete(r.entries, targetURL)
	}
	r.mu.Unlock()

	result, err := r.base.Fetch(ctx, targetURL)
	if err != nil {
		return webFetchFetchResult{}, err
	}

	cacheEntry := webFetchCacheEntry{
		Result: webFetchFetchResult{
			Document: result.Document,
			Cached:   false,
		},
		ExpiresAt: now.Add(r.ttl),
		LastUsed:  now,
	}

	r.mu.Lock()
	r.entries[targetURL] = cacheEntry
	r.evictLocked(now)
	r.mu.Unlock()

	return webFetchFetchResult{
		Document: result.Document,
		Cached:   false,
	}, nil
}

func (r *cachedWebFetchRepository) evictLocked(now time.Time) {
	for key, entry := range r.entries {
		if !now.Before(entry.ExpiresAt) {
			delete(r.entries, key)
		}
	}
	if len(r.entries) <= r.capacity {
		return
	}

	var (
		oldestKey string
		oldestAt  time.Time
	)
	for key, entry := range r.entries {
		if oldestKey == "" || entry.LastUsed.Before(oldestAt) {
			oldestKey = key
			oldestAt = entry.LastUsed
		}
	}
	if oldestKey != "" {
		delete(r.entries, oldestKey)
	}
}

func (r *httpWebFetchRepository) Fetch(ctx context.Context, targetURL string) (webFetchFetchResult, error) {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return webFetchFetchResult{}, &webFetchError{Code: model.ErrorCodeInvalidArgument, Message: "invalid url"}
	}
	if err := validateWebFetchTarget(ctx, parsed, r.allowPrivateHosts); err != nil {
		return webFetchFetchResult{}, classifyWebFetchError(err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return webFetchFetchResult{}, err
	}
	req.Header.Set("User-Agent", webSearchUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain,application/json,application/xml;q=0.9,*/*;q=0.5")

	resp, err := r.client.Do(req)
	if err != nil {
		return webFetchFetchResult{}, classifyWebFetchError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return webFetchFetchResult{}, classifyWebFetchStatus(resp.StatusCode)
	}

	body, err := readLimitedBody(resp.Body, r.bodyLimitBytes)
	if err != nil {
		return webFetchFetchResult{}, &webFetchError{Code: model.ErrorCodeInternal, Message: err.Error()}
	}

	document, err := buildWebFetchDocument(targetURL, resp.Request.URL, resp.Header.Get("Content-Type"), resp.StatusCode, body)
	if err != nil {
		return webFetchFetchResult{}, err
	}

	return webFetchFetchResult{Document: document}, nil
}

func buildWebFetchDocument(requestURL string, finalURL *url.URL, rawContentType string, statusCode int, body []byte) (webFetchDocument, error) {
	contentType := normalizeWebFetchContentType(rawContentType, body)
	if !isSupportedWebFetchContentType(contentType) {
		return webFetchDocument{}, &webFetchError{
			Code:    model.ErrorCodeInvalidArgument,
			Message: fmt.Sprintf("unsupported content type %q", contentType),
		}
	}

	text := extractWebFetchText(contentType, string(body))
	title := ""
	if contentType == "text/html" || contentType == "application/xhtml+xml" {
		title = extractWebFetchTitle(string(body))
	}
	if text == "" && title != "" {
		text = title
	}

	finalURLString := requestURL
	if finalURL != nil {
		finalURLString = finalURL.String()
	}

	return webFetchDocument{
		URL:         requestURL,
		FinalURL:    finalURLString,
		StatusCode:  statusCode,
		ContentType: contentType,
		Title:       title,
		Text:        text,
	}, nil
}

func normalizeWebFetchURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", errors.New("invalid url")
	}
	if parsed.User != nil {
		return "", errors.New("userinfo is not allowed")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("url must use http or https")
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return "", errors.New("url host is required")
	}
	parsed.Scheme = scheme
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String(), nil
}

func validateWebFetchTarget(ctx context.Context, target *url.URL, allowPrivateHosts bool) error {
	if target == nil {
		return &webFetchError{Code: model.ErrorCodeInvalidArgument, Message: "invalid url"}
	}
	scheme := strings.ToLower(strings.TrimSpace(target.Scheme))
	if scheme != "http" && scheme != "https" {
		return &webFetchError{Code: model.ErrorCodeInvalidArgument, Message: "url must use http or https"}
	}
	return validateWebFetchHost(ctx, target.Hostname(), allowPrivateHosts)
}

func validateWebFetchHost(ctx context.Context, host string, allowPrivateHosts bool) error {
	trimmed := strings.TrimSpace(strings.TrimSuffix(host, "."))
	if trimmed == "" {
		return &webFetchError{Code: model.ErrorCodeInvalidArgument, Message: "url host is required"}
	}
	if allowPrivateHosts {
		return nil
	}
	lower := strings.ToLower(trimmed)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return &webFetchError{Code: model.ErrorCodeForbidden, Message: "private or loopback hosts are not allowed"}
	}
	if addr, err := netip.ParseAddr(lower); err == nil {
		if !isPublicWebFetchAddr(addr) {
			return &webFetchError{Code: model.ErrorCodeForbidden, Message: "private or loopback hosts are not allowed"}
		}
		return nil
	}

	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", trimmed)
	if err != nil {
		return fmt.Errorf("resolve host: %w", err)
	}
	if len(ips) == 0 {
		return &webFetchError{Code: model.ErrorCodeNotFound, Message: "host did not resolve to any address"}
	}
	for _, ip := range ips {
		if !isPublicWebFetchAddr(ip) {
			return &webFetchError{Code: model.ErrorCodeForbidden, Message: "private or loopback hosts are not allowed"}
		}
	}
	return nil
}

func isPublicWebFetchAddr(addr netip.Addr) bool {
	return addr.IsValid() &&
		addr.IsGlobalUnicast() &&
		!addr.IsPrivate() &&
		!addr.IsLoopback() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsLinkLocalMulticast() &&
		!addr.IsMulticast() &&
		!addr.IsUnspecified()
}

func classifyWebFetchStatus(statusCode int) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return &webFetchError{Code: model.ErrorCodeUnauthorized, Message: fmt.Sprintf("unexpected status %d", statusCode)}
	case http.StatusForbidden:
		return &webFetchError{Code: model.ErrorCodeForbidden, Message: fmt.Sprintf("unexpected status %d", statusCode)}
	case http.StatusNotFound:
		return &webFetchError{Code: model.ErrorCodeNotFound, Message: fmt.Sprintf("unexpected status %d", statusCode)}
	case http.StatusTooManyRequests:
		return &webFetchError{Code: model.ErrorCodeRateLimited, Message: fmt.Sprintf("unexpected status %d", statusCode)}
	default:
		return &webFetchError{Code: model.ErrorCodeInternal, Message: fmt.Sprintf("unexpected status %d", statusCode)}
	}
}

func classifyWebFetchError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var fetchErr *webFetchError
	if errors.As(err, &fetchErr) {
		return err
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if nested := classifyWebFetchError(urlErr.Err); nested != nil {
			return nested
		}
	}
	return err
}

func normalizeWebFetchContentType(raw string, body []byte) string {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(raw))
	if err == nil && mediaType != "" {
		return strings.ToLower(mediaType)
	}
	return strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(body), ";")[0]))
}

func isSupportedWebFetchContentType(contentType string) bool {
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	switch contentType {
	case "application/xhtml+xml", "application/json", "application/xml", "text/xml":
		return true
	default:
		return false
	}
}

func extractWebFetchTitle(body string) string {
	match := reHTMLTitle.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return cleanHTMLText(match[1])
}

func extractWebFetchText(contentType, body string) string {
	switch contentType {
	case "text/html", "application/xhtml+xml":
		cleaned := reHTMLDropBlocks.ReplaceAllString(body, " ")
		cleaned = reHTMLLineBreaks.ReplaceAllString(cleaned, "\n")
		cleaned = reHTMLBlockEnds.ReplaceAllString(cleaned, "\n")
		cleaned = reHTMLTags.ReplaceAllString(cleaned, " ")
		cleaned = html.UnescapeString(cleaned)
		return normalizeWhitespace(cleaned)
	default:
		return strings.TrimSpace(body)
	}
}

func normalizeWebFetchMaxChars(v int) int {
	switch {
	case v <= 0:
		return defaultWebFetchMaxChars
	case v < minWebFetchMaxChars:
		return minWebFetchMaxChars
	case v > maxWebFetchMaxChars:
		return maxWebFetchMaxChars
	default:
		return v
	}
}

func resolveWebFetchMaxChars(raw any, fallback int) int {
	maxAllowed := normalizeWebFetchMaxChars(fallback)
	switch value := raw.(type) {
	case int:
		return minInt(normalizeWebFetchMaxChars(value), maxAllowed)
	case int32:
		return minInt(normalizeWebFetchMaxChars(int(value)), maxAllowed)
	case int64:
		return minInt(normalizeWebFetchMaxChars(int(value)), maxAllowed)
	case float64:
		return minInt(normalizeWebFetchMaxChars(int(value)), maxAllowed)
	default:
		return maxAllowed
	}
}

func normalizeWebFetchBodyLimit(v int64) int64 {
	if v <= 0 {
		return defaultWebFetchBodyLimit
	}
	return v
}

func normalizeWebFetchCacheCapacity(v int) int {
	if v <= 0 {
		return defaultWebFetchCacheCapacity
	}
	return v
}
