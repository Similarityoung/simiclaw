package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
	store "github.com/similarityyoung/simiclaw/pkg/persistence"
)

func TestSmokeM2_QueryChain(t *testing.T) {
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
		Conversation:   model.Conversation{ConversationID: "smoke_m2", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:smoke_m2:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "smoke m2"},
	}
	resp := ingest(t, app, req, 202)
	rec := pollEvent(t, app, resp.EventID)
	if rec.Status != model.EventStatusCommitted {
		t.Fatalf("expected committed, got %+v", rec)
	}

	paths := []string{
		"/v1/events?session_key=" + resp.SessionKey,
		"/v1/events:lookup?idempotency_key=cli:smoke_m2:1",
		"/v1/runs?session_key=" + resp.SessionKey,
		"/v1/runs/" + rec.RunID,
		"/v1/runs/" + rec.RunID + "/trace?view=summary",
		"/v1/sessions?conversation_id=smoke_m2",
		"/v1/sessions/" + resp.SessionKey,
	}
	for _, path := range paths {
		body, code := doRequest(t, app, http.MethodGet, path, nil)
		if code != 200 {
			t.Fatalf("GET %s expected 200, got %d body=%s", path, code, string(body))
		}
	}
}

func TestSmokeM2_RecoveryAfterTailCorruption(t *testing.T) {
	workspace := t.TempDir()
	app := newE2EApp(t, workspace)
	app.Start()

	req := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "smoke_m2_recover", ChannelType: "dm", ParticipantID: "u2"},
		IdempotencyKey: "cli:smoke_m2_recover:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "first"},
	}
	resp := ingest(t, app, req, 202)
	rec := pollEvent(t, app, resp.EventID)
	app.Stop()

	sessionPath := filepath.Join(workspace, "runtime", "sessions", rec.SessionID+".jsonl")
	if err := store.AppendJSONL(sessionPath, model.SessionEntry{
		Type:    "assistant",
		EntryID: "e_smoke_tail",
		RunID:   "run_smoke_tail",
		Content: "tail",
	}); err != nil {
		t.Fatalf("append broken tail: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "runtime", "sessions.json"), []byte("{invalid"), 0o644); err != nil {
		t.Fatalf("corrupt sessions.json: %v", err)
	}

	restarted := newE2EApp(t, workspace)
	restarted.Start()
	defer restarted.Stop()

	req2 := req
	req2.IdempotencyKey = "cli:smoke_m2_recover:2"
	req2.Timestamp = time.Now().UTC().Format(time.RFC3339)
	req2.Payload.Text = "second"
	resp2 := ingest(t, restarted, req2, 202)
	rec2 := pollEvent(t, restarted, resp2.EventID)
	if rec2.Status != model.EventStatusCommitted {
		t.Fatalf("expected committed after recovery, got %+v", rec2)
	}

	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("read session file: %v", err)
	}
	if strings.Contains(string(data), "e_smoke_tail") {
		t.Fatalf("tail entry should be rolled back")
	}
}

func newE2EApp(t *testing.T, workspace string) *api.App {
	t.Helper()
	cfg := config.Default()
	cfg.Workspace = workspace
	app, err := api.NewApp(cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	return app
}
