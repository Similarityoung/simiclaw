//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func newTestApp(t *testing.T, startLoops bool, queueCap int) *api.App {
	t.Helper()
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	cfg.EventQueueCapacity = queueCap
	cfg.IngestEnqueueTimeout = config.Duration{Duration: 60 * time.Millisecond}
	app, err := api.NewApp(cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	if startLoops {
		app.Start()
	}
	t.Cleanup(app.Stop)
	return app
}

func TestIngestLifecycleAndCommitOrder(t *testing.T) {
	app := newTestApp(t, true, 16)
	req := model.IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: "conv_a",
			ChannelType:    "dm",
			ParticipantID:  "u1",
		},
		IdempotencyKey: "cli:conv_a:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}
	resp, code := postIngest(t, app, req)
	if code != 202 {
		t.Fatalf("expected 202, got %d", code)
	}
	rec := waitEvent(t, app, resp.EventID, 2*time.Second)
	if rec.Status != model.EventStatusCommitted {
		t.Fatalf("expected committed, got %s", rec.Status)
	}
	if rec.DeliveryStatus != model.DeliveryStatusSent {
		t.Fatalf("expected sent, got %s", rec.DeliveryStatus)
	}
	if rec.RunID == "" || rec.CommitID == "" {
		t.Fatalf("run_id/commit_id should not be empty")
	}

	order := app.StoreLoop.OrderRecord()
	if len(order) < 3 {
		t.Fatalf("unexpected order record: %#v", order)
	}
	if order[0] != "append_batch" || order[1] != "write_run" || order[2] != "update_sessions" {
		t.Fatalf("invalid commit order: %#v", order)
	}
}

func TestDuplicateAndConflict(t *testing.T) {
	app := newTestApp(t, true, 16)
	now := time.Now().UTC().Format(time.RFC3339)
	base := model.IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: "conv_b",
			ChannelType:    "dm",
			ParticipantID:  "u1",
		},
		IdempotencyKey: "cli:conv_b:1",
		Timestamp:      now,
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}
	_, code := postIngest(t, app, base)
	if code != 202 {
		t.Fatalf("expected 202, got %d", code)
	}
	dupResp, code := postIngest(t, app, base)
	if code != 200 {
		t.Fatalf("expected 200 duplicate, got %d", code)
	}
	if dupResp.Status != "duplicate_acked" {
		t.Fatalf("expected duplicate_acked, got %s", dupResp.Status)
	}

	conflict := base
	conflict.Payload.Text = "changed"
	body, code := postIngestRaw(t, app, conflict)
	if code != 409 {
		t.Fatalf("expected 409, got %d, body=%s", code, string(body))
	}
}

func TestADKSessionRoutingDMParticipantSemantics(t *testing.T) {
	app := newTestApp(t, true, 16)
	now := time.Now().UTC().Format(time.RFC3339)

	req1 := model.IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: "conv_adk_session",
			ChannelType:    "dm",
			ParticipantID:  "u1",
		},
		IdempotencyKey: "cli:conv_adk_session:1",
		Timestamp:      now,
		Payload:        model.EventPayload{Type: "message", Text: "first"},
	}
	resp1, code := postIngest(t, app, req1)
	if code != 202 {
		t.Fatalf("expected first request 202, got %d", code)
	}

	req2 := req1
	req2.IdempotencyKey = "cli:conv_adk_session:2"
	req2.Timestamp = time.Now().UTC().Format(time.RFC3339)
	req2.Payload.Text = "second"
	resp2, code := postIngest(t, app, req2)
	if code != 202 {
		t.Fatalf("expected second request 202, got %d", code)
	}

	if resp1.SessionKey != resp2.SessionKey {
		t.Fatalf("expected same participant to reuse session key, got %q vs %q", resp1.SessionKey, resp2.SessionKey)
	}
	if resp1.ActiveSessionID != resp2.ActiveSessionID {
		t.Fatalf("expected same participant to reuse active session id, got %q vs %q", resp1.ActiveSessionID, resp2.ActiveSessionID)
	}

	req3 := req1
	req3.IdempotencyKey = "cli:conv_adk_session:3"
	req3.Timestamp = time.Now().UTC().Format(time.RFC3339)
	req3.Conversation.ParticipantID = "u2"
	req3.Payload.Text = "third"
	resp3, code := postIngest(t, app, req3)
	if code != 202 {
		t.Fatalf("expected third request 202, got %d", code)
	}

	if resp1.SessionKey == resp3.SessionKey {
		t.Fatalf("expected different participant to produce different session key, both are %q", resp1.SessionKey)
	}
	if resp1.ActiveSessionID == resp3.ActiveSessionID {
		t.Fatalf("expected different participant to produce different active session id, both are %q", resp1.ActiveSessionID)
	}
}

func TestQueueFull(t *testing.T) {
	app := newTestApp(t, false, 1)
	now := time.Now().UTC().Format(time.RFC3339)
	first := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv_q", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv_q:1",
		Timestamp:      now,
		Payload:        model.EventPayload{Type: "message", Text: "one"},
	}
	_, code := postIngest(t, app, first)
	if code != 202 {
		t.Fatalf("first expected 202, got %d", code)
	}
	second := first
	second.IdempotencyKey = "cli:conv_q:2"
	body, code := postIngestRaw(t, app, second)
	if code != 202 {
		t.Fatalf("expected 202 with direct ADK execution, got %d, body=%s", code, string(body))
	}
}

func TestQueueFullRetryWithSameIdempotencyKey(t *testing.T) {
	app := newTestApp(t, false, 1)
	now := time.Now().UTC().Format(time.RFC3339)

	first := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv_q_retry", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv_q_retry:1",
		Timestamp:      now,
		Payload:        model.EventPayload{Type: "message", Text: "one"},
	}
	firstResp, code := postIngest(t, app, first)
	if code != 202 {
		t.Fatalf("first expected 202, got %d", code)
	}

	second := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv_q_retry", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv_q_retry:2",
		Timestamp:      now,
		Payload:        model.EventPayload{Type: "message", Text: "two"},
	}
	body, code := postIngestRaw(t, app, second)
	if code != 202 {
		t.Fatalf("second expected 202 with direct ADK execution, got %d, body=%s", code, string(body))
	}

	_ = waitEvent(t, app, firstResp.EventID, 2*time.Second)

	second.Timestamp = time.Now().UTC().Format(time.RFC3339)
	secondResp, code := postIngest(t, app, second)
	if code != 200 {
		t.Fatalf("retry expected duplicate 200, got %d", code)
	}
	if secondResp.Status != "duplicate_acked" {
		t.Fatalf("expected duplicate_acked after retry, got %s", secondResp.Status)
	}
}

func TestRetryAfterEventPersistFailure(t *testing.T) {
	app := newTestApp(t, false, 16)
	eventsDir := filepath.Join(app.Cfg.Workspace, "runtime", "events")
	if err := os.Chmod(eventsDir, 0o500); err != nil {
		t.Skipf("chmod not supported for this env: %v", err)
	}
	defer func() { _ = os.Chmod(eventsDir, 0o755) }()

	req := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv_persist_fail", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv_persist_fail:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "fail then retry"},
	}
	_, code := postIngestRaw(t, app, req)
	if code != 500 {
		t.Fatalf("expected 500 on persist failure, got %d", code)
	}

	if err := os.Chmod(eventsDir, 0o755); err != nil {
		t.Fatalf("restore events dir permission: %v", err)
	}
	req.Timestamp = time.Now().UTC().Format(time.RFC3339)
	_, code = postIngest(t, app, req)
	if code != 202 {
		t.Fatalf("expected 202 after retry, got %d", code)
	}
}

func TestStopDrainsQueuedEvents(t *testing.T) {
	app := newTestApp(t, false, 16)
	eventIDs := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		req := model.IngestRequest{
			Source:         "cli",
			Conversation:   model.Conversation{ConversationID: "conv_stop", ChannelType: "dm", ParticipantID: "u1"},
			IdempotencyKey: fmt.Sprintf("cli:conv_stop:%d", i+1),
			Timestamp:      time.Now().UTC().Format(time.RFC3339),
			Payload:        model.EventPayload{Type: "message", Text: fmt.Sprintf("msg-%d", i+1)},
		}
		resp, code := postIngest(t, app, req)
		if code != 202 {
			t.Fatalf("ingest %d expected 202, got %d", i+1, code)
		}
		eventIDs = append(eventIDs, resp.EventID)
	}

	app.Start()
	app.Stop()

	for _, eventID := range eventIDs {
		rec, ok := app.Events.Get(eventID)
		if !ok {
			t.Fatalf("event not found after stop: %s", eventID)
		}
		if rec.Status == model.EventStatusAccepted || rec.Status == model.EventStatusRunning {
			t.Fatalf("event should be drained before stop returns: id=%s status=%s", eventID, rec.Status)
		}
	}
}

func TestNoReplySuppressed(t *testing.T) {
	app := newTestApp(t, true, 16)
	req := model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv_nr", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv_nr:1",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "memory_flush", Text: ""},
	}
	resp, code := postIngest(t, app, req)
	if code != 202 {
		t.Fatalf("expected 202, got %d", code)
	}
	rec := waitEvent(t, app, resp.EventID, 2*time.Second)
	if rec.DeliveryStatus != model.DeliveryStatusSuppressed {
		t.Fatalf("expected suppressed, got %s", rec.DeliveryStatus)
	}
	if rec.RunMode != model.RunModeNoReply {
		t.Fatalf("expected NO_REPLY, got %s", rec.RunMode)
	}
}

func postIngest(t *testing.T, app *api.App, req model.IngestRequest) (model.IngestResponse, int) {
	t.Helper()
	var resp model.IngestResponse
	body, status := postIngestRaw(t, app, req)
	if len(body) > 0 {
		_ = json.Unmarshal(body, &resp)
	}
	return resp, status
}

func postIngestRaw(t *testing.T, app *api.App, req model.IngestRequest) ([]byte, int) {
	t.Helper()
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal req: %v", err)
	}
	return doRequest(t, app, http.MethodPost, "/v1/events:ingest", b)
}

func waitEvent(t *testing.T, app *api.App, eventID string, timeout time.Duration) model.EventRecord {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		rec, ok := fetchEvent(t, app, eventID)
		if ok && rec.Status == model.EventStatusFailed {
			return rec
		}
		if ok && rec.Status == model.EventStatusCommitted &&
			rec.DeliveryStatus != model.DeliveryStatusPending &&
			rec.DeliveryStatus != model.DeliveryStatusNotApplicable {
			return rec
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting event: %s", eventID)
	return model.EventRecord{}
}

func fetchEvent(t *testing.T, app *api.App, eventID string) (model.EventRecord, bool) {
	t.Helper()
	body, code := doRequest(t, app, http.MethodGet, fmt.Sprintf("/v1/events/%s", eventID), nil)
	if code != 200 {
		return model.EventRecord{}, false
	}
	var rec model.EventRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	return rec, true
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
