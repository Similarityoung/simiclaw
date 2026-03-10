package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

const (
	defaultPollInterval = 100 * time.Millisecond
	defaultPollTimeout  = 60 * time.Second
)

type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e.Code != "" && e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("http status %d", e.StatusCode)
}

type StreamRecoverableError struct {
	EventID string
	Err     error
}

func (e *StreamRecoverableError) Error() string {
	if e == nil || e.Err == nil {
		return "stream interrupted"
	}
	if e.EventID == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("stream interrupted for %s: %s", e.EventID, e.Err)
}

func (e *StreamRecoverableError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

var ErrStreamUnsupported = errors.New("streaming unsupported")
var ErrStreamProtocolMismatch = errors.New("stream protocol mismatch")

type Client struct {
	baseURL          string
	apiKey           string
	httpClient       *http.Client
	streamHTTPClient *http.Client
	requestTimeout   time.Duration
	pollInterval     time.Duration
	pollTimeout      time.Duration
}

type SessionPage struct {
	Items      []api.SessionRecord `json:"items"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

type MessagePage struct {
	Items      []api.MessageRecord `json:"items"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

type EventListItem struct {
	EventID      string             `json:"event_id"`
	Status       model.EventStatus  `json:"status"`
	OutboxStatus model.OutboxStatus `json:"outbox_status,omitempty"`
	SessionKey   string             `json:"session_key"`
	SessionID    string             `json:"session_id"`
	RunID        string             `json:"run_id,omitempty"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

type EventPage struct {
	Items      []EventListItem `json:"items"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

type RunSummary struct {
	RunID      string          `json:"run_id"`
	EventID    string          `json:"event_id"`
	SessionKey string          `json:"session_key"`
	SessionID  string          `json:"session_id"`
	RunMode    model.RunMode   `json:"run_mode"`
	Status     model.RunStatus `json:"status"`
	StartedAt  time.Time       `json:"started_at"`
	EndedAt    time.Time       `json:"ended_at"`
}

type RunPage struct {
	Items      []RunSummary `json:"items"`
	NextCursor string       `json:"next_cursor,omitempty"`
}

type HealthReport struct {
	Health map[string]any `json:"health"`
	Ready  map[string]any `json:"ready"`
}

func New(baseURL, apiKey string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		baseURL:          strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:           strings.TrimSpace(apiKey),
		httpClient:       &http.Client{Timeout: timeout},
		streamHTTPClient: newStreamHTTPClient(timeout),
		requestTimeout:   timeout,
		pollInterval:     defaultPollInterval,
		pollTimeout:      defaultPollTimeout,
	}
}

func (c *Client) Health(ctx context.Context) (HealthReport, error) {
	var health map[string]any
	if err := c.getJSON(ctx, c.httpClient, "/healthz", nil, &health); err != nil {
		return HealthReport{}, err
	}
	var ready map[string]any
	if err := c.getJSON(ctx, c.httpClient, "/readyz", nil, &ready); err != nil {
		return HealthReport{}, err
	}
	return HealthReport{Health: health, Ready: ready}, nil
}

func (c *Client) Ingest(ctx context.Context, req api.IngestRequest) (api.IngestResponse, error) {
	var out api.IngestResponse
	if err := c.postJSON(ctx, "/v1/events:ingest", req, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) StreamChat(ctx context.Context, req api.IngestRequest, onEvent func(api.ChatStreamEvent) error) (api.EventRecord, error) {
	rec, err := c.streamChat(ctx, req, onEvent)
	if err == nil {
		return rec, nil
	}
	if errors.Is(err, ErrStreamUnsupported) {
		return c.ingestAndWait(ctx, req, onEvent)
	}
	var recoverable *StreamRecoverableError
	if errors.As(err, &recoverable) && recoverable.EventID != "" {
		rec, waitErr := c.WaitEvent(ctx, recoverable.EventID)
		if waitErr != nil {
			return api.EventRecord{}, waitErr
		}
		if onEvent != nil {
			if cbErr := onEvent(eventFromRecord(rec)); cbErr != nil {
				return api.EventRecord{}, cbErr
			}
		}
		return rec, nil
	}
	return api.EventRecord{}, err
}

func (c *Client) streamChat(ctx context.Context, req api.IngestRequest, onEvent func(api.ChatStreamEvent) error) (api.EventRecord, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return api.EventRecord{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat:stream", bytes.NewReader(body))
	if err != nil {
		return api.EventRecord{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.streamHTTPClient.Do(httpReq)
	if err != nil {
		return api.EventRecord{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if isStreamFallbackStatus(resp.StatusCode) {
			return api.EventRecord{}, ErrStreamUnsupported
		}
		return api.EventRecord{}, decodeAPIError(resp)
	}
	if !strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return api.EventRecord{}, ErrStreamUnsupported
	}

	reader := bufio.NewReader(resp.Body)
	var acceptedEventID string
	for {
		var (
			eventType string
			data      []byte
		)
		if acceptedEventID == "" {
			eventType, data, err = readSSEEventWithTimeout(reader, resp.Body, c.requestTimeout)
		} else {
			eventType, data, err = readSSEEvent(reader)
		}
		if err != nil {
			if acceptedEventID != "" {
				return api.EventRecord{}, &StreamRecoverableError{EventID: acceptedEventID, Err: err}
			}
			return api.EventRecord{}, err
		}
		var event api.ChatStreamEvent
		if err := json.Unmarshal(data, &event); err != nil {
			if acceptedEventID != "" {
				return api.EventRecord{}, &StreamRecoverableError{EventID: acceptedEventID, Err: err}
			}
			return api.EventRecord{}, err
		}
		if eventType != string(event.Type) {
			err = fmt.Errorf("stream event type mismatch: header=%s payload=%s", eventType, event.Type)
			if acceptedEventID != "" {
				return api.EventRecord{}, &StreamRecoverableError{EventID: acceptedEventID, Err: err}
			}
			return api.EventRecord{}, err
		}
		if event.Type == api.ChatStreamEventAccepted {
			acceptedEventID = event.EventID
			if onEvent != nil {
				if err := onEvent(event); err != nil {
					return api.EventRecord{}, err
				}
			}
			if event.StreamProtocolVersion != api.ChatStreamProtocolVersion {
				return api.EventRecord{}, &StreamRecoverableError{EventID: acceptedEventID, Err: ErrStreamProtocolMismatch}
			}
			continue
		}
		if onEvent != nil {
			if err := onEvent(event); err != nil {
				return api.EventRecord{}, err
			}
		}
		if !event.IsTerminal() {
			continue
		}
		if event.EventRecord != nil {
			if !isTerminalEvent(*event.EventRecord) {
				return c.WaitEvent(ctx, event.EventRecord.EventID)
			}
			return *event.EventRecord, nil
		}
		if event.Error != nil {
			return api.EventRecord{}, &APIError{StatusCode: http.StatusOK, Code: event.Error.Code, Message: event.Error.Message}
		}
		return api.EventRecord{}, errors.New("stream terminal event missing event_record")
	}
}

func (c *Client) ingestAndWait(ctx context.Context, req api.IngestRequest, onEvent func(api.ChatStreamEvent) error) (api.EventRecord, error) {
	resp, err := c.Ingest(ctx, req)
	if err != nil {
		return api.EventRecord{}, err
	}
	if onEvent != nil {
		accepted := api.ChatStreamEvent{
			Type:                  api.ChatStreamEventAccepted,
			EventID:               resp.EventID,
			At:                    time.Now().UTC(),
			StreamProtocolVersion: api.ChatStreamProtocolVersion,
			IngestResponse:        &resp,
		}
		if err := onEvent(accepted); err != nil {
			return api.EventRecord{}, err
		}
	}
	rec, err := c.WaitEvent(ctx, resp.EventID)
	if err != nil {
		return api.EventRecord{}, err
	}
	if onEvent != nil {
		if err := onEvent(eventFromRecord(rec)); err != nil {
			return api.EventRecord{}, err
		}
	}
	return rec, nil
}

func (c *Client) GetEvent(ctx context.Context, eventID string) (api.EventRecord, error) {
	var out api.EventRecord
	if err := c.getJSON(ctx, c.httpClient, "/v1/events/"+url.PathEscape(eventID), nil, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) WaitEvent(ctx context.Context, eventID string) (api.EventRecord, error) {
	deadline := time.Time{}
	if c.pollTimeout > 0 {
		deadline = time.Now().Add(c.pollTimeout)
	}
	for {
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return api.EventRecord{}, fmt.Errorf("poll timeout after %s", c.pollTimeout)
		}
		rec, err := c.GetEvent(ctx, eventID)
		if err != nil {
			return api.EventRecord{}, err
		}
		if isTerminalEvent(rec) {
			return rec, nil
		}
		select {
		case <-ctx.Done():
			return api.EventRecord{}, ctx.Err()
		case <-time.After(c.pollInterval):
		}
	}
}

func (c *Client) ListSessions(ctx context.Context, sessionKey, conversationID, cursor string, limit int) (SessionPage, error) {
	values := url.Values{}
	if sessionKey != "" {
		values.Set("session_key", sessionKey)
	}
	if conversationID != "" {
		values.Set("conversation_id", conversationID)
	}
	if cursor != "" {
		values.Set("cursor", cursor)
	}
	if limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", limit))
	}
	var out SessionPage
	if err := c.getJSON(ctx, c.httpClient, "/v1/sessions", values, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) GetSession(ctx context.Context, sessionKey string) (api.SessionRecord, error) {
	var out api.SessionRecord
	if err := c.getJSON(ctx, c.httpClient, "/v1/sessions/"+url.PathEscape(sessionKey), nil, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) GetSessionHistory(ctx context.Context, sessionKey, cursor string, limit int, visibleOnly bool) (MessagePage, error) {
	values := url.Values{}
	if cursor != "" {
		values.Set("cursor", cursor)
	}
	if limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", limit))
	}
	if !visibleOnly {
		values.Set("visible", "false")
	}
	var out MessagePage
	if err := c.getJSON(ctx, c.httpClient, "/v1/sessions/"+url.PathEscape(sessionKey)+"/history", values, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) ListEvents(ctx context.Context, sessionKey string, status model.EventStatus, cursor string, limit int) (EventPage, error) {
	values := url.Values{}
	if sessionKey != "" {
		values.Set("session_key", sessionKey)
	}
	if status != "" {
		values.Set("status", string(status))
	}
	if cursor != "" {
		values.Set("cursor", cursor)
	}
	if limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", limit))
	}
	var out EventPage
	if err := c.getJSON(ctx, c.httpClient, "/v1/events", values, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) ListRuns(ctx context.Context, sessionKey, cursor string, limit int) (RunPage, error) {
	values := url.Values{}
	if sessionKey != "" {
		values.Set("session_key", sessionKey)
	}
	if cursor != "" {
		values.Set("cursor", cursor)
	}
	if limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", limit))
	}
	var out RunPage
	if err := c.getJSON(ctx, c.httpClient, "/v1/runs", values, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) GetRunTrace(ctx context.Context, runID string) (api.RunTrace, error) {
	var out api.RunTrace
	if err := c.getJSON(ctx, c.httpClient, "/v1/runs/"+url.PathEscape(runID)+"/trace", nil, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) getJSON(ctx context.Context, httpClient *http.Client, path string, values url.Values, out any) error {
	fullURL := c.baseURL + path
	if len(values) > 0 {
		fullURL += "?" + values.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) postJSON(ctx context.Context, path string, reqBody any, out any) error {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func newStreamHTTPClient(responseHeaderTimeout time.Duration) *http.Client {
	if responseHeaderTimeout <= 0 {
		return &http.Client{}
	}
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Client{}
	}
	cloned := transport.Clone()
	cloned.ResponseHeaderTimeout = responseHeaderTimeout
	return &http.Client{Transport: cloned}
}

func decodeAPIError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return &APIError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("http status %d", resp.StatusCode)}
	}
	var parsed api.ErrorResponse
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error.Code != "" {
		return &APIError{StatusCode: resp.StatusCode, Code: parsed.Error.Code, Message: parsed.Error.Message}
	}
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = fmt.Sprintf("http status %d", resp.StatusCode)
	}
	return &APIError{StatusCode: resp.StatusCode, Message: message}
}

func isTerminalEvent(rec api.EventRecord) bool {
	switch rec.Status {
	case model.EventStatusSuppressed, model.EventStatusFailed:
		return true
	case model.EventStatusProcessed:
		return rec.OutboxStatus == "" || rec.OutboxStatus == model.OutboxStatusSent || rec.OutboxStatus == model.OutboxStatusDead
	default:
		return false
	}
}

func eventFromRecord(rec api.EventRecord) api.ChatStreamEvent {
	eventType := api.ChatStreamEventDone
	if rec.Status == model.EventStatusFailed {
		eventType = api.ChatStreamEventError
	}
	return api.ChatStreamEvent{
		Type:        eventType,
		EventID:     rec.EventID,
		At:          time.Now().UTC(),
		EventRecord: &rec,
		Error:       rec.Error,
	}
}

func isStreamFallbackStatus(status int) bool {
	return status == http.StatusNotFound ||
		status == http.StatusMethodNotAllowed ||
		status == http.StatusNotImplemented ||
		status == http.StatusBadGateway
}

type sseReadResult struct {
	eventType string
	data      []byte
	err       error
}

func readSSEEventWithTimeout(r *bufio.Reader, closer io.Closer, timeout time.Duration) (string, []byte, error) {
	if timeout <= 0 {
		return readSSEEvent(r)
	}
	resultCh := make(chan sseReadResult, 1)
	go func() {
		eventType, data, err := readSSEEvent(r)
		resultCh <- sseReadResult{eventType: eventType, data: data, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case result := <-resultCh:
		return result.eventType, result.data, result.err
	case <-timer.C:
		if closer != nil {
			_ = closer.Close()
		}
		return "", nil, fmt.Errorf("stream handshake timeout after %s: %w", timeout, context.DeadlineExceeded)
	}
}

func readSSEEvent(r *bufio.Reader) (string, []byte, error) {
	var (
		eventType string
		data      []byte
	)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", nil, err
		}
		if line == "\n" {
			if eventType == "" && len(data) == 0 {
				continue
			}
			return eventType, data, nil
		}
		if len(line) > 0 && line[0] == ':' {
			continue
		}
		switch {
		case strings.HasPrefix(line, "event: "):
			eventType = strings.TrimRight(line[len("event: "):], "\r\n")
		case strings.HasPrefix(line, "data: "):
			part := strings.TrimRight(line[len("data: "):], "\r\n")
			if data == nil {
				data = []byte(part)
				continue
			}
			data = append(data, '\n')
			data = append(data, part...)
		}
	}
}
