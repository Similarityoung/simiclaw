package ingest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/similarityyoung/simiclaw/internal/testutil/logcapture"
	"github.com/similarityyoung/simiclaw/pkg/logging"
)

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
	if !strings.Contains(line, " WARN ") {
		t.Fatalf("expected WARN level log, got %q", line)
	}
	logcapture.AssertContainsInOrder(t, line,
		"error_code=INVALID_ARGUMENT",
		"method=POST",
		"path=/v1/events:ingest",
		"status_code=400",
	)
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
	if !strings.Contains(line, " WARN ") {
		t.Fatalf("expected WARN level log, got %q", line)
	}
	logcapture.AssertContainsInOrder(t, line,
		"error_code=INVALID_ARGUMENT",
		`message="field timestamp is required"`,
		"method=POST",
		"path=/v1/events:ingest",
		"status_code=400",
	)
}
