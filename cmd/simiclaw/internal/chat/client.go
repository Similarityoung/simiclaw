package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
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

type HTTPClient struct {
	baseURL        string
	apiKey         string
	httpClient     *http.Client
	requestTimeout time.Duration
	pollInterval   time.Duration
	pollTimeout    time.Duration
}

type StreamEventHandler interface {
	HandleStreamEvent(event model.ChatStreamEvent) error
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

func NewHTTPClient(baseURL, apiKey string, requestTimeout, pollInterval, pollTimeout time.Duration) *HTTPClient {
	return &HTTPClient{
		baseURL:        strings.TrimRight(baseURL, "/"),
		apiKey:         strings.TrimSpace(apiKey),
		httpClient:     &http.Client{},
		requestTimeout: requestTimeout,
		pollInterval:   pollInterval,
		pollTimeout:    pollTimeout,
	}
}

func (c *HTTPClient) SendAndWait(ctx context.Context, req model.IngestRequest) (model.EventRecord, error) {
	ingestResp, err := c.ingest(ctx, req)
	if err != nil {
		return model.EventRecord{}, err
	}
	return c.pollEvent(ctx, ingestResp.EventID)
}

func (c *HTTPClient) SendStream(ctx context.Context, req model.IngestRequest, handler StreamEventHandler) (model.EventRecord, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return model.EventRecord{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat:stream", bytes.NewReader(body))
	if err != nil {
		return model.EventRecord{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return model.EventRecord{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if isStreamFallbackStatus(resp.StatusCode) {
			return model.EventRecord{}, ErrStreamUnsupported
		}
		return model.EventRecord{}, decodeAPIError(resp)
	}
	if !strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return model.EventRecord{}, ErrStreamUnsupported
	}

	reader := bufio.NewReader(resp.Body)
	var acceptedEventID string
	for {
		eventType, data, err := readSSEEvent(reader)
		if err != nil {
			if acceptedEventID != "" {
				return model.EventRecord{}, &StreamRecoverableError{EventID: acceptedEventID, Err: err}
			}
			return model.EventRecord{}, err
		}
		var event model.ChatStreamEvent
		if err := json.Unmarshal(data, &event); err != nil {
			if acceptedEventID != "" {
				return model.EventRecord{}, &StreamRecoverableError{EventID: acceptedEventID, Err: err}
			}
			return model.EventRecord{}, err
		}
		if eventType != string(event.Type) {
			err = fmt.Errorf("stream event type mismatch: header=%s payload=%s", eventType, event.Type)
			if acceptedEventID != "" {
				return model.EventRecord{}, &StreamRecoverableError{EventID: acceptedEventID, Err: err}
			}
			return model.EventRecord{}, err
		}
		if event.Type == model.ChatStreamEventAccepted {
			acceptedEventID = event.EventID
			if handler != nil {
				if err := handler.HandleStreamEvent(event); err != nil {
					return model.EventRecord{}, err
				}
			}
			if event.StreamProtocolVersion != model.ChatStreamProtocolVersion {
				return model.EventRecord{}, &StreamRecoverableError{
					EventID: acceptedEventID,
					Err:     ErrStreamProtocolMismatch,
				}
			}
			continue
		}
		if handler != nil {
			if err := handler.HandleStreamEvent(event); err != nil {
				return model.EventRecord{}, err
			}
		}
		if event.Type == model.ChatStreamEventDone || event.Type == model.ChatStreamEventError {
			if event.EventRecord != nil {
				return *event.EventRecord, nil
			}
			if event.Error != nil {
				return model.EventRecord{}, &APIError{StatusCode: http.StatusOK, Code: event.Error.Code, Message: event.Error.Message}
			}
			return model.EventRecord{}, errors.New("stream terminal event missing event_record")
		}
	}
}

func (c *HTTPClient) PollEvent(ctx context.Context, eventID string) (model.EventRecord, error) {
	return c.pollEvent(ctx, eventID)
}

func (c *HTTPClient) ingest(ctx context.Context, req model.IngestRequest) (model.IngestResponse, error) {
	var out model.IngestResponse
	body, err := json.Marshal(req)
	if err != nil {
		return out, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.baseURL+"/v1/events:ingest", bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return out, decodeAPIError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	if out.EventID == "" {
		return out, errors.New("missing event_id in ingest response")
	}
	return out, nil
}

func (c *HTTPClient) pollEvent(ctx context.Context, eventID string) (model.EventRecord, error) {
	deadline := time.Now().Add(c.pollTimeout)
	for {
		if !time.Now().Before(deadline) {
			return model.EventRecord{}, fmt.Errorf("poll timeout after %s", c.pollTimeout)
		}
		rec, err := c.getEvent(ctx, eventID)
		if err != nil {
			return model.EventRecord{}, err
		}
		if rec.Status == model.EventStatusFailed {
			return rec, nil
		}
		if isTerminalEvent(rec) {
			return rec, nil
		}

		select {
		case <-ctx.Done():
			return model.EventRecord{}, ctx.Err()
		case <-time.After(c.pollInterval):
		}
	}
}

func (c *HTTPClient) getEvent(ctx context.Context, eventID string) (model.EventRecord, error) {
	var rec model.EventRecord
	reqCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, c.baseURL+"/v1/events/"+url.PathEscape(eventID), nil)
	if err != nil {
		return rec, err
	}
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return rec, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return rec, decodeAPIError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(&rec); err != nil {
		return rec, err
	}
	return rec, nil
}

func isTerminalEvent(rec model.EventRecord) bool {
	switch rec.Status {
	case model.EventStatusSuppressed:
		return true
	case model.EventStatusProcessed:
		return rec.OutboxStatus == "" ||
			rec.OutboxStatus == model.OutboxStatusSent ||
			rec.OutboxStatus == model.OutboxStatusDead
	case model.EventStatusFailed:
		return true
	default:
		return false
	}
}

func decodeAPIError(resp *http.Response) error {
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return &APIError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("http status %d", resp.StatusCode)}
	}

	var parsed model.ErrorResponse
	if err := json.Unmarshal(b, &parsed); err == nil && parsed.Error.Code != "" {
		return &APIError{StatusCode: resp.StatusCode, Code: parsed.Error.Code, Message: parsed.Error.Message}
	}
	msg := strings.TrimSpace(string(b))
	if msg == "" {
		msg = fmt.Sprintf("http status %d", resp.StatusCode)
	}
	return &APIError{StatusCode: resp.StatusCode, Message: msg}
}

func isStreamFallbackStatus(status int) bool {
	return status == http.StatusNotFound ||
		status == http.StatusMethodNotAllowed ||
		status == http.StatusNotImplemented ||
		status == http.StatusBadGateway
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
