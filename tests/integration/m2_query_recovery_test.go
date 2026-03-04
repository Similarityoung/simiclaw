//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
	"github.com/similarityyoung/simiclaw/pkg/store"
)

func TestM2EventsListAndLookup(t *testing.T) {
	app := newTestApp(t, true, 32)
	req1 := model.IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: "conv_m2_events",
			ChannelType:    "dm",
			ParticipantID:  "u1",
		},
		IdempotencyKey: "cli:conv_m2_events:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "hello-1"},
	}
	req2 := req1
	req2.IdempotencyKey = "cli:conv_m2_events:2"
	req2.Timestamp = time.Now().UTC().Format(time.RFC3339)
	req2.Payload.Text = "hello-2"

	resp1, code := postIngest(t, app, req1)
	if code != 202 {
		t.Fatalf("ingest-1 expected 202, got %d", code)
	}
	resp2, code := postIngest(t, app, req2)
	if code != 202 {
		t.Fatalf("ingest-2 expected 202, got %d", code)
	}
	_ = waitEvent(t, app, resp1.EventID, 2*time.Second)
	_ = waitEvent(t, app, resp2.EventID, 2*time.Second)

	body, code := doRequest(t, app, http.MethodGet, "/v1/events?session_key="+resp1.SessionKey+"&limit=1", nil)
	if code != 200 {
		t.Fatalf("list events expected 200, got %d, body=%s", code, string(body))
	}
	var listResp struct {
		Items []struct {
			EventID string `json:"event_id"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal(body, &listResp); err != nil {
		t.Fatalf("decode list events: %v", err)
	}
	if len(listResp.Items) != 1 || listResp.NextCursor == "" {
		t.Fatalf("expected one item + next_cursor, got %+v", listResp)
	}

	body, code = doRequest(t, app, http.MethodGet, "/v1/events?session_key="+resp1.SessionKey+"&limit=1&cursor="+listResp.NextCursor, nil)
	if code != 200 {
		t.Fatalf("list events by cursor expected 200, got %d, body=%s", code, string(body))
	}

	body, code = doRequest(t, app, http.MethodGet, "/v1/events:lookup?idempotency_key=cli:conv_m2_events:1", nil)
	if code != 200 {
		t.Fatalf("events lookup expected 200, got %d, body=%s", code, string(body))
	}
	var lookup struct {
		EventID string `json:"event_id"`
	}
	if err := json.Unmarshal(body, &lookup); err != nil {
		t.Fatalf("decode lookup: %v", err)
	}
	if lookup.EventID != resp1.EventID {
		t.Fatalf("lookup event_id mismatch: got=%s want=%s", lookup.EventID, resp1.EventID)
	}

	_, code = doRequest(t, app, http.MethodGet, "/v1/events:lookup?idempotency_key=cli:conv_m2_events:404", nil)
	if code != 404 {
		t.Fatalf("lookup missing key expected 404, got %d", code)
	}
}

func TestM2RunsAndSessionsQuery(t *testing.T) {
	app := newTestApp(t, true, 32)
	req := model.IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: "conv_m2_runs",
			ChannelType:    "dm",
			ParticipantID:  "u2",
		},
		IdempotencyKey: "cli:conv_m2_runs:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "hello run"},
	}
	resp, code := postIngest(t, app, req)
	if code != 202 {
		t.Fatalf("ingest expected 202, got %d", code)
	}
	rec := waitEvent(t, app, resp.EventID, 2*time.Second)
	if rec.RunID == "" {
		t.Fatalf("run_id should not be empty")
	}

	body, code := doRequest(t, app, http.MethodGet, "/v1/runs?session_key="+resp.SessionKey, nil)
	if code != 200 {
		t.Fatalf("list runs expected 200, got %d, body=%s", code, string(body))
	}
	var listRuns struct {
		Items []struct {
			RunID string `json:"run_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &listRuns); err != nil {
		t.Fatalf("decode runs list: %v", err)
	}
	if len(listRuns.Items) == 0 {
		t.Fatalf("runs list should not be empty")
	}

	body, code = doRequest(t, app, http.MethodGet, "/v1/runs/"+rec.RunID, nil)
	if code != 200 {
		t.Fatalf("get run expected 200, got %d, body=%s", code, string(body))
	}

	body, code = doRequest(t, app, http.MethodGet, "/v1/runs/"+rec.RunID+"/trace?view=summary", nil)
	if code != 200 {
		t.Fatalf("get run trace summary expected 200, got %d, body=%s", code, string(body))
	}
	body, code = doRequest(t, app, http.MethodGet, "/v1/runs/"+rec.RunID+"/trace?redact=true", nil)
	if code != 200 {
		t.Fatalf("get run trace redact expected 200, got %d, body=%s", code, string(body))
	}

	body, code = doRequest(t, app, http.MethodGet, "/v1/sessions?conversation_id=conv_m2_runs", nil)
	if code != 200 {
		t.Fatalf("list sessions expected 200, got %d, body=%s", code, string(body))
	}
	var listSessions struct {
		Items []struct {
			SessionKey   string `json:"session_key"`
			LastCommitID string `json:"last_commit_id"`
			LastRunID    string `json:"last_run_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &listSessions); err != nil {
		t.Fatalf("decode sessions list: %v", err)
	}
	if len(listSessions.Items) == 0 {
		t.Fatalf("sessions list should not be empty")
	}
	if listSessions.Items[0].LastCommitID == "" || listSessions.Items[0].LastRunID == "" {
		t.Fatalf("session progress fields should be populated: %+v", listSessions.Items[0])
	}

	body, code = doRequest(t, app, http.MethodGet, "/v1/sessions/"+resp.SessionKey, nil)
	if code != 200 {
		t.Fatalf("get session expected 200, got %d, body=%s", code, string(body))
	}
}

func TestM2RecoveryRebuildAndTailRollback(t *testing.T) {
	workspace := t.TempDir()
	app := newAppOnWorkspace(t, workspace, true)

	req := model.IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: "conv_m2_recover",
			ChannelType:    "dm",
			ParticipantID:  "u3",
		},
		IdempotencyKey: "cli:conv_m2_recover:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "first"},
	}
	resp, code := postIngest(t, app, req)
	if code != 202 {
		t.Fatalf("ingest expected 202, got %d", code)
	}
	rec := waitEvent(t, app, resp.EventID, 2*time.Second)
	if rec.Status != model.EventStatusCommitted {
		t.Fatalf("expected committed before restart, got %s", rec.Status)
	}
	app.Stop()

	sessionPath := filepath.Join(workspace, "runtime", "sessions", rec.SessionID+".jsonl")
	if err := store.AppendJSONL(sessionPath, model.SessionEntry{
		Type:    "assistant",
		EntryID: "e_tail_uncommitted",
		RunID:   "run_tail_uncommitted",
		Content: "broken tail",
	}); err != nil {
		t.Fatalf("append tail: %v", err)
	}
	sessionsPath := filepath.Join(workspace, "runtime", "sessions.json")
	if err := os.WriteFile(sessionsPath, []byte("{invalid"), 0o644); err != nil {
		t.Fatalf("corrupt sessions.json: %v", err)
	}

	restarted := newAppOnWorkspace(t, workspace, true)
	defer restarted.Stop()

	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("read session file: %v", err)
	}
	if strings.Contains(string(data), "e_tail_uncommitted") {
		t.Fatalf("tail rollback failed, uncommitted entry still exists")
	}

	req2 := req
	req2.IdempotencyKey = "cli:conv_m2_recover:2"
	req2.Timestamp = time.Now().UTC().Format(time.RFC3339)
	req2.Payload.Text = "second"
	resp2, code := postIngest(t, restarted, req2)
	if code != 202 {
		t.Fatalf("ingest after recovery expected 202, got %d", code)
	}
	rec2 := waitEvent(t, restarted, resp2.EventID, 2*time.Second)
	if rec2.Status != model.EventStatusCommitted {
		t.Fatalf("expected committed after recovery, got %s", rec2.Status)
	}

	body, code := doRequest(t, restarted, http.MethodGet, "/v1/sessions/"+resp.SessionKey, nil)
	if code != 200 {
		t.Fatalf("get session after recovery expected 200, got %d, body=%s", code, string(body))
	}
}

func newAppOnWorkspace(t *testing.T, workspace string, start bool) *api.App {
	t.Helper()
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.EventQueueCapacity = 32
	cfg.IngestEnqueueTimeout = config.Duration{Duration: 80 * time.Millisecond}
	app, err := api.NewApp(cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	if start {
		app.Start()
	}
	return app
}

func TestM2SessionsJSONMissingCanRebuild(t *testing.T) {
	workspace := t.TempDir()
	app := newAppOnWorkspace(t, workspace, true)

	req := model.IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: "conv_m2_missing",
			ChannelType:    "dm",
			ParticipantID:  "u4",
		},
		IdempotencyKey: "cli:conv_m2_missing:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}
	resp, code := postIngest(t, app, req)
	if code != 202 {
		t.Fatalf("ingest expected 202, got %d", code)
	}
	_ = waitEvent(t, app, resp.EventID, 2*time.Second)
	app.Stop()

	sessionsPath := filepath.Join(workspace, "runtime", "sessions.json")
	if err := os.Remove(sessionsPath); err != nil {
		t.Fatalf("remove sessions.json: %v", err)
	}

	restarted := newAppOnWorkspace(t, workspace, true)
	defer restarted.Stop()
	body, code := doRequest(t, restarted, http.MethodGet, "/v1/sessions/"+resp.SessionKey, nil)
	if code != 200 {
		t.Fatalf("rebuilt sessions index expected 200, got %d, body=%s", code, string(body))
	}
}

func TestM2CursorValidation(t *testing.T) {
	app := newTestApp(t, true, 16)
	endpoints := []string{
		"/v1/events?cursor=bad",
		"/v1/runs?cursor=bad",
		"/v1/sessions?cursor=bad",
	}
	for _, ep := range endpoints {
		body, code := doRequest(t, app, http.MethodGet, ep, nil)
		if code != 400 {
			t.Fatalf("endpoint %s expected 400, got %d body=%s", ep, code, string(body))
		}
	}
}

func TestM2RunTraceViewValidation(t *testing.T) {
	app := newTestApp(t, true, 16)
	req := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv_m2_view", ChannelType: "dm", ParticipantID: "u5"},
		IdempotencyKey: "cli:conv_m2_view:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}
	resp, code := postIngest(t, app, req)
	if code != 202 {
		t.Fatalf("ingest expected 202, got %d", code)
	}
	rec := waitEvent(t, app, resp.EventID, 2*time.Second)
	body, code := doRequest(t, app, http.MethodGet, fmt.Sprintf("/v1/runs/%s/trace?view=invalid", rec.RunID), nil)
	if code != 400 {
		t.Fatalf("invalid view expected 400, got %d body=%s", code, string(body))
	}
}
