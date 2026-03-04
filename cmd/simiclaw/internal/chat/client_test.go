package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestHTTPClientSendAndWaitSuccessWithAPIKey(t *testing.T) {
	polls := 0
	c := NewHTTPClient("http://unit.test", "secret", time.Second, 10*time.Millisecond, time.Second)
	c.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.Method == http.MethodPost && req.URL.Path == "/v1/events:ingest":
				if got := req.Header.Get("Authorization"); got != "Bearer secret" {
					t.Fatalf("unexpected auth header: %s", got)
				}
				return jsonResponse(http.StatusAccepted, model.IngestResponse{EventID: "evt_1"}), nil
			case req.Method == http.MethodGet && req.URL.Path == "/v1/events/evt_1":
				polls++
				if polls == 1 {
					return jsonResponse(http.StatusOK, model.EventRecord{
						EventID:        "evt_1",
						Status:         model.EventStatusRunning,
						DeliveryStatus: model.DeliveryStatusNotApplicable,
					}), nil
				}
				return jsonResponse(http.StatusOK, model.EventRecord{
					EventID:        "evt_1",
					Status:         model.EventStatusCommitted,
					DeliveryStatus: model.DeliveryStatusSent,
					AssistantReply: "ok",
				}), nil
			default:
				t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
				return nil, nil
			}
		}),
	}

	rec, err := c.SendAndWait(context.Background(), model.IngestRequest{Source: "cli"})
	if err != nil {
		t.Fatalf("SendAndWait: %v", err)
	}
	if rec.AssistantReply != "ok" {
		t.Fatalf("unexpected assistant reply: %q", rec.AssistantReply)
	}
}

func TestHTTPClientSendAndWaitFailedEvent(t *testing.T) {
	c := NewHTTPClient("http://unit.test", "", time.Second, 10*time.Millisecond, time.Second)
	c.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.Method == http.MethodPost && req.URL.Path == "/v1/events:ingest":
				return jsonResponse(http.StatusAccepted, model.IngestResponse{EventID: "evt_2"}), nil
			case req.Method == http.MethodGet && req.URL.Path == "/v1/events/evt_2":
				return jsonResponse(http.StatusOK, model.EventRecord{
					EventID:        "evt_2",
					Status:         model.EventStatusFailed,
					DeliveryStatus: model.DeliveryStatusFailed,
					Error:          &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: "runner failed"},
				}), nil
			default:
				t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
				return nil, nil
			}
		}),
	}

	rec, err := c.SendAndWait(context.Background(), model.IngestRequest{Source: "cli"})
	if err != nil {
		t.Fatalf("SendAndWait: %v", err)
	}
	if rec.Status != model.EventStatusFailed {
		t.Fatalf("unexpected status: %s", rec.Status)
	}
}

func TestHTTPClientSendAndWaitTimeout(t *testing.T) {
	c := NewHTTPClient("http://unit.test", "", time.Second, 10*time.Millisecond, 40*time.Millisecond)
	c.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.Method == http.MethodPost && req.URL.Path == "/v1/events:ingest":
				return jsonResponse(http.StatusAccepted, model.IngestResponse{EventID: "evt_3"}), nil
			case req.Method == http.MethodGet && req.URL.Path == "/v1/events/evt_3":
				return jsonResponse(http.StatusOK, model.EventRecord{
					EventID:        "evt_3",
					Status:         model.EventStatusRunning,
					DeliveryStatus: model.DeliveryStatusPending,
				}), nil
			default:
				t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
				return nil, nil
			}
		}),
	}

	_, err := c.SendAndWait(context.Background(), model.IngestRequest{Source: "cli"})
	if err == nil || !stringsContains(err.Error(), "poll timeout") {
		t.Fatalf("expected poll timeout, got %v", err)
	}
}

func TestHTTPClientSendAndWaitNoReply(t *testing.T) {
	c := NewHTTPClient("http://unit.test", "", time.Second, 10*time.Millisecond, time.Second)
	c.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.Method == http.MethodPost && req.URL.Path == "/v1/events:ingest":
				return jsonResponse(http.StatusAccepted, model.IngestResponse{EventID: "evt_4"}), nil
			case req.Method == http.MethodGet && req.URL.Path == "/v1/events/evt_4":
				return jsonResponse(http.StatusOK, model.EventRecord{
					EventID:        "evt_4",
					Status:         model.EventStatusCommitted,
					DeliveryStatus: model.DeliveryStatusSuppressed,
					RunMode:        model.RunModeNoReply,
				}), nil
			default:
				t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
				return nil, nil
			}
		}),
	}

	rec, err := c.SendAndWait(context.Background(), model.IngestRequest{Source: "cli"})
	if err != nil {
		t.Fatalf("SendAndWait: %v", err)
	}
	if rec.AssistantReply != "" {
		t.Fatalf("expected empty assistant reply, got %q", rec.AssistantReply)
	}
}

func TestHTTPClientIngestAPIError(t *testing.T) {
	c := NewHTTPClient("http://unit.test", "", time.Second, 10*time.Millisecond, time.Second)
	c.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusUnauthorized, model.ErrorResponse{
				Error: model.ErrorBlock{
					Code:    model.ErrorCodeUnauthorized,
					Message: "missing or invalid api key",
				},
			}), nil
		}),
	}

	_, err := c.SendAndWait(context.Background(), model.IngestRequest{Source: "cli"})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Code != model.ErrorCodeUnauthorized {
		t.Fatalf("unexpected api error code: %s", apiErr.Code)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, v any) *http.Response {
	b, _ := json.Marshal(v)
	b = append(b, '\n')
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(b)),
	}
}

func stringsContains(s, sub string) bool {
	return bytes.Contains([]byte(s), []byte(sub))
}
