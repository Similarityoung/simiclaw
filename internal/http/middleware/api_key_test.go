package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/similarityyoung/simiclaw/internal/testutil/logcapture"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestWithAPIKeyRejectsMissingBearerToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/secured", nil)
	rec := httptest.NewRecorder()
	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		handler := WithAPIKey("secret", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		handler.ServeHTTP(rec, req)
		_ = logging.Sync()
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body api.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != model.ErrorCodeUnauthorized {
		t.Fatalf("expected unauthorized error code, got %+v", body.Error)
	}
	line := logcapture.FirstNonEmptyLine(t, out)
	if !strings.Contains(line, "[http.auth] api key rejected") {
		t.Fatalf("unexpected log line: %q", line)
	}
	logcapture.AssertContainsInOrder(t, line,
		"error_code=UNAUTHORIZED",
		"method=GET",
		`path=/secured`,
		"status_code=401",
	)
}

func TestWithAPIKeyAllowsValidBearerToken(t *testing.T) {
	handler := WithAPIKey("secret", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/secured", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected pass-through status, got %d body=%s", rec.Code, rec.Body.String())
	}
}
