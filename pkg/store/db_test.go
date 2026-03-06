package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestInitWorkspaceRejectsLegacy(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "runtime"), 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "runtime", "sessions.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}
	if err := InitWorkspace(workspace, false, DefaultBusyTimeout()); err == nil {
		t.Fatalf("expected legacy workspace rejection")
	}
	if err := InitWorkspace(workspace, true, DefaultBusyTimeout()); err != nil {
		t.Fatalf("force init workspace: %v", err)
	}
	if _, err := os.Stat(DBPath(workspace)); err != nil {
		t.Fatalf("expected app.db to exist: %v", err)
	}
}

func TestClaimEventSingleActiveRun(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	now := time.Now().UTC()
	result, err := db.IngestEvent(ctx, "local", "local:dm:u1", model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}, "sha256:test", now)
	if err != nil {
		t.Fatalf("ingest event: %v", err)
	}
	if err := db.MarkEventQueued(ctx, result.EventID, now); err != nil {
		t.Fatalf("mark queued: %v", err)
	}
	if _, ok, err := db.ClaimEvent(ctx, result.EventID, "run_1", now); err != nil || !ok {
		t.Fatalf("first claim failed ok=%v err=%v", ok, err)
	}
	if _, ok, err := db.ClaimEvent(ctx, result.EventID, "run_2", now.Add(time.Second)); err != nil {
		t.Fatalf("second claim err: %v", err)
	} else if ok {
		t.Fatalf("expected second claim to be rejected")
	}
}

func TestFinalizeRunUpdatesFTSAndSessionAggregate(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	now := time.Now().UTC()
	result, err := db.IngestEvent(ctx, "local", "local:dm:u1", model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello alpha"},
	}, "sha256:test", now)
	if err != nil {
		t.Fatalf("ingest event: %v", err)
	}
	if err := db.MarkEventQueued(ctx, result.EventID, now); err != nil {
		t.Fatalf("mark queued: %v", err)
	}
	claimed, ok, err := db.ClaimEvent(ctx, result.EventID, "run_1", now)
	if err != nil || !ok {
		t.Fatalf("claim event ok=%v err=%v", ok, err)
	}
	if err := db.FinalizeRun(ctx, RunFinalize{
		RunID:             claimed.RunID,
		EventID:           claimed.Event.EventID,
		SessionKey:        claimed.Event.SessionKey,
		SessionID:         claimed.Event.ActiveSessionID,
		RunMode:           model.RunModeNormal,
		RunStatus:         model.RunStatusCompleted,
		EventStatus:       model.EventStatusProcessed,
		Provider:          "fake",
		Model:             "default",
		PromptTokens:      8,
		CompletionTokens:  8,
		TotalTokens:       16,
		LatencyMS:         12,
		FinishReason:      "stop",
		RawFinishReason:   "stop",
		ProviderRequestID: "fake-request-1",
		OutputText:        "已收到: hello alpha",
		AssistantReply:    "已收到: hello alpha",
		OutboxBody:        "已收到: hello alpha",
		Messages: []StoredMessage{
			{MessageID: "msg_user", SessionKey: claimed.Event.SessionKey, SessionID: claimed.Event.ActiveSessionID, RunID: claimed.RunID, Role: "user", Content: "hello alpha", Visible: true, CreatedAt: now},
			{MessageID: "msg_assistant", SessionKey: claimed.Event.SessionKey, SessionID: claimed.Event.ActiveSessionID, RunID: claimed.RunID, Role: "assistant", Content: "已收到: hello alpha", Visible: true, CreatedAt: now},
		},
		Now: now,
	}); err != nil {
		t.Fatalf("finalize run: %v", err)
	}

	hits, err := db.SearchMessagesFTS(ctx, claimed.Event.ActiveSessionID, "hello", 5)
	if err != nil {
		t.Fatalf("search fts: %v", err)
	}
	if len(hits) == 0 {
		t.Fatalf("expected FTS hits")
	}

	session, ok, err := db.GetSession(ctx, claimed.Event.SessionKey)
	if err != nil || !ok {
		t.Fatalf("get session ok=%v err=%v", ok, err)
	}
	if session.MessageCount != 2 {
		t.Fatalf("expected message_count=2, got %d", session.MessageCount)
	}
	if session.TotalTokensTotal != 16 {
		t.Fatalf("expected total_tokens_total=16, got %d", session.TotalTokensTotal)
	}
	if session.LastRunID != claimed.RunID {
		t.Fatalf("expected last_run_id=%s, got %s", claimed.RunID, session.LastRunID)
	}
}

func TestRecoverExpiredProcessingAndOutboxLease(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	now := time.Now().UTC()
	result, err := db.IngestEvent(ctx, "local", "local:dm:u1", model.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello retry"},
	}, "sha256:test", now)
	if err != nil {
		t.Fatalf("ingest event: %v", err)
	}
	if err := db.MarkEventQueued(ctx, result.EventID, now); err != nil {
		t.Fatalf("mark queued: %v", err)
	}
	claimed, ok, err := db.ClaimEvent(ctx, result.EventID, "run_1", now)
	if err != nil || !ok {
		t.Fatalf("claim event ok=%v err=%v", ok, err)
	}
	ids, err := db.RecoverExpiredProcessing(ctx, now.Add(time.Second), now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("recover expired processing: %v", err)
	}
	if len(ids) != 1 || ids[0] != result.EventID {
		t.Fatalf("expected requeued event %s, got %+v", result.EventID, ids)
	}
	eventRec, ok, err := db.GetEvent(ctx, result.EventID)
	if err != nil || !ok {
		t.Fatalf("get event ok=%v err=%v", ok, err)
	}
	if eventRec.Status != model.EventStatusQueued {
		t.Fatalf("expected queued after recovery, got %s", eventRec.Status)
	}
	runTrace, ok, err := db.GetRun(ctx, claimed.RunID)
	if err != nil || !ok {
		t.Fatalf("get run ok=%v err=%v", ok, err)
	}
	if runTrace.Status != model.RunStatusFailed {
		t.Fatalf("expected failed run after lease expiry, got %s", runTrace.Status)
	}

	if _, ok, err := db.ClaimEvent(ctx, result.EventID, "run_2", now.Add(3*time.Second)); err != nil || !ok {
		t.Fatalf("reclaim event ok=%v err=%v", ok, err)
	}
	if err := db.FinalizeRun(ctx, RunFinalize{
		RunID:          "run_2",
		EventID:        result.EventID,
		SessionKey:     claimed.Event.SessionKey,
		SessionID:      claimed.Event.ActiveSessionID,
		RunMode:        model.RunModeNormal,
		RunStatus:      model.RunStatusCompleted,
		EventStatus:    model.EventStatusProcessed,
		AssistantReply: "[fail_outbound]",
		OutboxBody:     "[fail_outbound]",
		Messages: []StoredMessage{
			{MessageID: "msg_user", SessionKey: claimed.Event.SessionKey, SessionID: claimed.Event.ActiveSessionID, RunID: "run_2", Role: "user", Content: "hello retry", Visible: true, CreatedAt: now},
		},
		Now: now.Add(3 * time.Second),
	}); err != nil {
		t.Fatalf("finalize second run: %v", err)
	}
	outbox, ok, err := db.ClaimOutbox(ctx, "test-worker", now.Add(4*time.Second))
	if err != nil || !ok {
		t.Fatalf("claim outbox ok=%v err=%v", ok, err)
	}
	if err := db.RecoverExpiredSending(ctx, now.Add(5*time.Second), now.Add(6*time.Second)); err != nil {
		t.Fatalf("recover expired sending: %v", err)
	}
	eventRec, ok, err = db.GetEvent(ctx, result.EventID)
	if err != nil || !ok {
		t.Fatalf("get event after outbox recover ok=%v err=%v", ok, err)
	}
	if eventRec.OutboxStatus != model.OutboxStatusRetryWait {
		t.Fatalf("expected retry_wait, got %s for outbox %s", eventRec.OutboxStatus, outbox.OutboxID)
	}
}

func newTestDB(t *testing.T) *DB {
	t.Helper()
	workspace := t.TempDir()
	if err := InitWorkspace(workspace, false, DefaultBusyTimeout()); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	db, err := Open(workspace, DefaultBusyTimeout())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}
