//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/bootstrap"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestIngestToProcessedAndQuerySQLite(t *testing.T) {
	app := newTestApp(t)

	req := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "integration", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:integration:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello sqlite"},
	}
	resp := ingest(t, app, req, http.StatusAccepted)
	event := pollEvent(t, app, resp.EventID)
	if event.Status != model.EventStatusProcessed {
		t.Fatalf("expected processed, got %+v", event)
	}
	if event.OutboxStatus != model.OutboxStatusSent {
		t.Fatalf("expected sent outbox, got %+v", event)
	}
	if event.AssistantReply != "已收到: hello sqlite" {
		t.Fatalf("unexpected assistant reply: %+v", event)
	}

	runTrace := getRunTrace(t, app, event.RunID)
	if runTrace.Provider != "fake" || runTrace.Model != "default" {
		t.Fatalf("unexpected run trace provider/model: %+v", runTrace)
	}
	if runTrace.TotalTokens == 0 {
		t.Fatalf("expected token usage in trace")
	}

	session := getSession(t, app, resp.SessionKey)
	if session.MessageCount != 2 {
		t.Fatalf("expected message_count=2, got %+v", session)
	}
}

func TestNoReplySuppressed(t *testing.T) {
	app := newTestApp(t)
	req := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "integration-no-reply", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:integration-no-reply:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "memory_flush", Text: "flush"},
	}
	resp := ingest(t, app, req, http.StatusAccepted)
	event := pollEvent(t, app, resp.EventID)
	if event.Status != model.EventStatusSuppressed {
		t.Fatalf("expected suppressed, got %+v", event)
	}
	if event.OutboxStatus != "" {
		t.Fatalf("expected no outbox status, got %+v", event)
	}
}

func TestReadyzRequiresDBAndEventLoop(t *testing.T) {
	app := newTestApp(t)
	body, code := doRequest(t, app, http.MethodGet, "/readyz", nil)
	if code != http.StatusOK {
		t.Fatalf("readyz expected 200, got %d body=%s", code, string(body))
	}
}

func newTestApp(t *testing.T) *bootstrap.App {
	t.Helper()
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	if err := store.InitWorkspace(cfg.Workspace, false, cfg.DBBusyTimeout.Duration); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	app, err := bootstrap.NewApp(cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	app.Start()
	t.Cleanup(app.Stop)
	return app
}

func ingest(t *testing.T, app *bootstrap.App, req model.IngestRequest, want int) model.IngestResponse {
	t.Helper()
	body, _ := json.Marshal(req)
	respBody, code := doRequest(t, app, http.MethodPost, "/v1/events:ingest", body)
	if code != want {
		t.Fatalf("expected %d got %d body=%s", want, code, string(respBody))
	}
	var resp model.IngestResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("decode ingest response: %v", err)
	}
	return resp
}

func pollEvent(t *testing.T, app *bootstrap.App, eventID string) model.EventRecord {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		body, code := doRequest(t, app, http.MethodGet, "/v1/events/"+eventID, nil)
		if code != http.StatusOK {
			t.Fatalf("event query expected 200, got %d body=%s", code, string(body))
		}
		var event model.EventRecord
		if err := json.Unmarshal(body, &event); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		if event.Status == model.EventStatusSuppressed || event.Status == model.EventStatusFailed {
			return event
		}
		if event.Status == model.EventStatusProcessed && (event.OutboxStatus == model.OutboxStatusSent || event.OutboxStatus == model.OutboxStatusDead) {
			return event
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout polling event %s", eventID)
	return model.EventRecord{}
}

func getRunTrace(t *testing.T, app *bootstrap.App, runID string) model.RunTrace {
	t.Helper()
	body, code := doRequest(t, app, http.MethodGet, "/v1/runs/"+runID+"/trace", nil)
	if code != http.StatusOK {
		t.Fatalf("run trace expected 200, got %d body=%s", code, string(body))
	}
	var trace model.RunTrace
	if err := json.Unmarshal(body, &trace); err != nil {
		t.Fatalf("decode run trace: %v", err)
	}
	return trace
}

func getSession(t *testing.T, app *bootstrap.App, sessionKey string) model.SessionRecord {
	t.Helper()
	body, code := doRequest(t, app, http.MethodGet, "/v1/sessions/"+sessionKey, nil)
	if code != http.StatusOK {
		t.Fatalf("session query expected 200, got %d body=%s", code, string(body))
	}
	var session model.SessionRecord
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	return session
}

func doRequest(t *testing.T, app *bootstrap.App, method, path string, body []byte) ([]byte, int) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	app.Handler.ServeHTTP(rr, req)
	return rr.Body.Bytes(), rr.Code
}
