package httpapi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/gateway"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestWithAuthRejectsMissingBearerToken(t *testing.T) {
	server := &Server{cfg: config.Config{APIKey: "secret"}}
	handler := server.withAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/secured", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d body=%s", rec.Code, rec.Body.String())
	}

	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected pass-through status, got %d", rec.Code)
	}
}

func TestWriteAPIErrorAndSSEHelpers(t *testing.T) {
	rec := httptest.NewRecorder()
	writeAPIError(rec, &gateway.APIError{
		StatusCode: http.StatusTooManyRequests,
		Code:       model.ErrorCodeRateLimited,
		Message:    "rate limited",
		RetryAfter: 3,
	})
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "3" {
		t.Fatalf("unexpected api error response: code=%d headers=%v body=%s", rec.Code, rec.Header(), rec.Body.String())
	}

	rec = httptest.NewRecorder()
	if err := writeSSEData(rec, map[string]string{"status": "ok"}); err != nil {
		t.Fatalf("writeSSEData: %v", err)
	}
	if err := writeSSEComment(rec, rec, "keepalive"); err != nil {
		t.Fatalf("writeSSEComment: %v", err)
	}
	if got := rec.Body.String(); got == "" {
		t.Fatalf("expected SSE payload")
	}
}

func TestReadSSEEventAndTrimLine(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(": comment\n")
	buf.WriteString("event: accepted\r\n")
	buf.WriteString("data: {\"a\":1}\n")
	buf.WriteString("data: {\"b\":2}\n\n")

	eventType, payload, err := readSSEEvent(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("readSSEEvent: %v", err)
	}
	if eventType != "accepted" || string(payload) != "{\"a\":1}\n{\"b\":2}" {
		t.Fatalf("unexpected SSE event type=%q payload=%q", eventType, string(payload))
	}
	if trimSSELine("hello\r\n") != "hello" {
		t.Fatalf("trimSSELine failed")
	}
}

func TestTerminalEventFromRecordVariants(t *testing.T) {
	failed := terminalEventFromRecord(model.EventRecord{
		EventID:   "evt_failed",
		Status:    model.EventStatusFailed,
		UpdatedAt: time.Now().UTC(),
		Error:     &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: "boom"},
	})
	if failed == nil || failed.Type != model.ChatStreamEventError {
		t.Fatalf("expected error terminal event, got %+v", failed)
	}
	done := terminalEventFromRecord(model.EventRecord{
		EventID:   "evt_done",
		Status:    model.EventStatusProcessed,
		UpdatedAt: time.Now().UTC(),
	})
	if done == nil || done.Type != model.ChatStreamEventDone {
		t.Fatalf("expected done terminal event, got %+v", done)
	}
	if terminalEventFromRecord(model.EventRecord{Status: model.EventStatusQueued}) != nil {
		t.Fatalf("expected nil terminal event for queued record")
	}
}

func TestWriteJSONProducesJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusCreated, map[string]string{"status": "ok"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("unexpected body: %+v", body)
	}
}
