package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
	"github.com/similarityyoung/simiclaw/internal/testutil/logcapture"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/logging"
)

type captureGateway struct {
	accepted gateway.AcceptedIngest
	got      gatewaymodel.NormalizedIngress
}

func (g *captureGateway) Accept(_ context.Context, in gatewaymodel.NormalizedIngress) (gateway.AcceptedIngest, *gateway.APIError) {
	g.got = in
	return g.accepted, nil
}

func TestHandleIngestDelegatesToGatewayCommandSeam(t *testing.T) {
	now := time.Date(2026, 3, 25, 9, 30, 0, 0, time.UTC)
	capture := &captureGateway{
		accepted: gateway.AcceptedIngest{
			Response: api.IngestResponse{
				EventID:         "evt_ingest",
				SessionKey:      "local:dm:u1",
				ActiveSessionID: "ses_ingest",
				ReceivedAt:      now.Format(time.RFC3339Nano),
				PayloadHash:     "hash_ingest",
				Status:          "accepted",
				StatusURL:       "/v1/events/evt_ingest",
			},
			StatusCode: http.StatusAccepted,
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/events:ingest", strings.NewReader(`{
		"source":"cli",
		"conversation":{"conversation_id":"conv-1","channel_type":"dm","participant_id":"u1"},
		"idempotency_key":"cli:conv-1:1",
		"timestamp":"2026-03-25T09:30:00Z",
		"payload":{"type":"message","text":"hello seam"}
	}`))
	rec := httptest.NewRecorder()

	NewHandler(capture).HandleIngest(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if capture.got.IdempotencyKey != "cli:conv-1:1" || capture.got.Payload.Text != "hello seam" {
		t.Fatalf("unexpected normalized ingress passed to gateway: %+v", capture.got)
	}
	var body api.IngestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.EventID != "evt_ingest" || body.SessionKey != "local:dm:u1" {
		t.Fatalf("unexpected ingest response body: %+v", body)
	}
}

func TestHandleIngestLogsDecodeFailure(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/events:ingest", strings.NewReader(`{"source":`))
	rec := httptest.NewRecorder()

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		handler := NewHandler(nil)
		handler.HandleIngest(rec, req)
		_ = logging.Sync()
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d", rec.Code)
	}
	line := logcapture.FirstNonEmptyLine(t, out)
	if !strings.Contains(line, "[http.ingest] request decode failed") {
		t.Fatalf("unexpected log line: %q", line)
	}
	if !strings.Contains(line, "\tWARN\t") {
		t.Fatalf("expected WARN level log, got %q", line)
	}
	for _, part := range []string{
		`"error_code": "INVALID_ARGUMENT"`,
		`"method": "POST"`,
		`"path": "/v1/events:ingest"`,
		`"status_code": 400`,
	} {
		if !strings.Contains(line, part) {
			t.Fatalf("missing %q in %q", part, line)
		}
	}
}

func TestHandleIngestLogsNormalizeFailure(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/events:ingest", strings.NewReader(`{"source":"cli"}`))
	rec := httptest.NewRecorder()

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		handler := NewHandler(nil)
		handler.HandleIngest(rec, req)
		_ = logging.Sync()
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d", rec.Code)
	}
	line := logcapture.FirstNonEmptyLine(t, out)
	if !strings.Contains(line, "[http.ingest] request normalize failed") {
		t.Fatalf("unexpected log line: %q", line)
	}
	if !strings.Contains(line, "\tWARN\t") {
		t.Fatalf("expected WARN level log, got %q", line)
	}
	for _, part := range []string{
		`"error_code": "INVALID_ARGUMENT"`,
		`"message": "field timestamp is required"`,
		`"method": "POST"`,
		`"path": "/v1/events:ingest"`,
		`"status_code": 400`,
	} {
		if !strings.Contains(line, part) {
			t.Fatalf("missing %q in %q", part, line)
		}
	}
}
