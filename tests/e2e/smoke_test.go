package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
	"github.com/similarityyoung/simiclaw/pkg/api"
)

func TestSmokeM1_IngestToSend(t *testing.T) {
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	app, err := api.NewApp(cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	app.Start()
	defer app.Stop()

	req := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "smoke", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:smoke:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "smoke"},
	}
	resp := ingest(t, app, req, 202)
	rec := pollEvent(t, app, resp.EventID)
	if rec.Status != model.EventStatusCommitted || rec.DeliveryStatus != model.DeliveryStatusSent {
		t.Fatalf("unexpected event state: %+v", rec)
	}
	if rec.AssistantReply != "已收到: smoke" {
		t.Fatalf("unexpected assistant_reply: %+v", rec)
	}
}

func TestSmokeM1_NoReply(t *testing.T) {
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	app, err := api.NewApp(cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	app.Start()
	defer app.Stop()

	req := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "smoke", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:smoke:2",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "memory_flush"},
	}
	resp := ingest(t, app, req, 202)
	rec := pollEvent(t, app, resp.EventID)
	if rec.DeliveryStatus != model.DeliveryStatusSuppressed {
		t.Fatalf("expected suppressed, got %+v", rec)
	}
	if rec.AssistantReply != "" {
		t.Fatalf("expected empty assistant_reply for no-reply event, got %+v", rec)
	}
}

func ingest(t *testing.T, app *api.App, req model.IngestRequest, want int) model.IngestResponse {
	t.Helper()
	b, _ := json.Marshal(req)
	body, code := doRequest(t, app, http.MethodPost, "/v1/events:ingest", b)
	if code != want {
		t.Fatalf("want %d got %d body=%s", want, code, string(body))
	}
	var resp model.IngestResponse
	_ = json.Unmarshal(body, &resp)
	return resp
}

func pollEvent(t *testing.T, app *api.App, eventID string) model.EventRecord {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		body, _ := doRequest(t, app, http.MethodGet, "/v1/events/"+eventID, nil)
		var rec model.EventRecord
		_ = json.Unmarshal(body, &rec)
		if rec.Status == model.EventStatusFailed {
			return rec
		}
		if rec.Status == model.EventStatusCommitted &&
			rec.DeliveryStatus != model.DeliveryStatusPending &&
			rec.DeliveryStatus != model.DeliveryStatusNotApplicable {
			return rec
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout polling event")
	return model.EventRecord{}
}

func doRequest(t *testing.T, app *api.App, method, path string, body []byte) ([]byte, int) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	app.Handler.ServeHTTP(rr, req)
	return rr.Body.Bytes(), rr.Code
}
