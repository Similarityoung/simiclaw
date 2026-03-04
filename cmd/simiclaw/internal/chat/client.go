package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	pollInterval time.Duration
	pollTimeout  time.Duration
}

func NewHTTPClient(baseURL, apiKey string, requestTimeout, pollInterval, pollTimeout time.Duration) *HTTPClient {
	return &HTTPClient{
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiKey:       strings.TrimSpace(apiKey),
		httpClient:   &http.Client{Timeout: requestTimeout},
		pollInterval: pollInterval,
		pollTimeout:  pollTimeout,
	}
}

func (c *HTTPClient) SendAndWait(ctx context.Context, req model.IngestRequest) (model.EventRecord, error) {
	ingestResp, err := c.ingest(ctx, req)
	if err != nil {
		return model.EventRecord{}, err
	}
	return c.pollEvent(ctx, ingestResp.EventID)
}

func (c *HTTPClient) ingest(ctx context.Context, req model.IngestRequest) (model.IngestResponse, error) {
	var out model.IngestResponse
	body, err := json.Marshal(req)
	if err != nil {
		return out, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/events:ingest", bytes.NewReader(body))
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
		rec, err := c.getEvent(ctx, eventID)
		if err != nil {
			return model.EventRecord{}, err
		}
		if rec.Status == model.EventStatusFailed {
			return rec, nil
		}
		if rec.Status == model.EventStatusCommitted && isTerminalDeliveryStatus(rec.DeliveryStatus) {
			return rec, nil
		}
		if time.Now().After(deadline) {
			return model.EventRecord{}, fmt.Errorf("poll timeout after %s", c.pollTimeout)
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
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/events/"+eventID, nil)
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

func isTerminalDeliveryStatus(status model.DeliveryStatus) bool {
	return status == model.DeliveryStatusSent ||
		status == model.DeliveryStatusSuppressed ||
		status == model.DeliveryStatusFailed
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
