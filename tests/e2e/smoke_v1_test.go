package e2e

import (
	"bytes"
	"encoding/json"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/bootstrap"
	"github.com/similarityyoung/simiclaw/internal/channels/cli"
	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestSmokeV1_InitServeChat(t *testing.T) {
	runSmokeV1(t)
}

// Backward-compatible alias for older stage-specific scripts.
func TestSmokeV1Alpha_InitServeChat(t *testing.T) {
	runSmokeV1(t)
}

func runSmokeV1(t *testing.T) {
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	if err := store.InitWorkspace(cfg.Workspace, false, cfg.DBBusyTimeout.Duration); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	app, err := bootstrap.NewApp(cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	if err := app.Start(); err != nil {
		t.Fatalf("start app: %v", err)
	}
	defer app.Stop()

	req := cli.BuildIngestRequest("smoke-v1", "u1", 1, "hello v1")
	resp := ingest(t, app, req)
	event := pollEvent(t, app, resp.EventID)
	if event.Status != model.EventStatusProcessed || event.OutboxStatus != model.OutboxStatusSent {
		t.Fatalf("unexpected terminal event: %+v", event)
	}
	if event.AssistantReply != "已收到: hello v1" {
		t.Fatalf("unexpected assistant reply: %+v", event)
	}
}

func ingest(t *testing.T, app *bootstrap.App, req api.IngestRequest) api.IngestResponse {
	t.Helper()
	body, _ := json.Marshal(req)
	respBody, code := doRequest(t, app, http.MethodPost, "/v1/events:ingest", body)
	if code != http.StatusAccepted {
		t.Fatalf("ingest expected 202, got %d body=%s", code, string(respBody))
	}
	var resp api.IngestResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("decode ingest response: %v", err)
	}
	return resp
}

func pollEvent(t *testing.T, app *bootstrap.App, eventID string) api.EventRecord {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		body, code := doRequest(t, app, http.MethodGet, "/v1/events/"+eventID, nil)
		if code != http.StatusOK {
			t.Fatalf("event query expected 200, got %d body=%s", code, string(body))
		}
		var event api.EventRecord
		if err := json.Unmarshal(body, &event); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		if event.Status == model.EventStatusProcessed && event.OutboxStatus == model.OutboxStatusSent {
			return event
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for event %s", eventID)
	return api.EventRecord{}
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
