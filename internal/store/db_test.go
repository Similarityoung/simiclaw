package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/api"

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
	result, err := db.IngestEvent(ctx, "local", "local:dm:u1", persistRequest(api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}), "sha256:test", now)
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
	result, err := db.IngestEvent(ctx, "local", "local:dm:u1", persistRequest(api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello alpha"},
	}), "sha256:test", now)
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

func TestFinalizeRunRecentMessagesRestoresAssistantToolCalls(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	now := time.Now().UTC()
	result, err := db.IngestEvent(ctx, "local", "local:dm:u1", persistRequest(api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv:tool:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello tool"},
	}), "sha256:test-tool", now)
	if err != nil {
		t.Fatalf("ingest event: %v", err)
	}
	if err := db.MarkEventQueued(ctx, result.EventID, now); err != nil {
		t.Fatalf("mark queued: %v", err)
	}
	claimed, ok, err := db.ClaimEvent(ctx, result.EventID, "run_tool_1", now)
	if err != nil || !ok {
		t.Fatalf("claim event ok=%v err=%v", ok, err)
	}
	toolCalls := []model.ToolCall{{ToolCallID: "call_1", Name: "memory_search", Args: map[string]any{"query": "hello tool"}}}
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
		OutputText:        "done",
		AssistantReply:    "done",
		OutboxBody:        "done",
		Messages: []StoredMessage{
			{MessageID: "msg_user", SessionKey: claimed.Event.SessionKey, SessionID: claimed.Event.ActiveSessionID, RunID: claimed.RunID, Role: "user", Content: "hello tool", Visible: true, CreatedAt: now},
			{MessageID: "msg_assistant_calls", SessionKey: claimed.Event.SessionKey, SessionID: claimed.Event.ActiveSessionID, RunID: claimed.RunID, Role: "assistant", Content: "", Visible: false, ToolCalls: toolCalls, CreatedAt: now},
			{MessageID: "msg_tool", SessionKey: claimed.Event.SessionKey, SessionID: claimed.Event.ActiveSessionID, RunID: claimed.RunID, Role: "tool", Content: `{"hits":[]}`, Visible: true, ToolCallID: "call_1", ToolName: "memory_search", CreatedAt: now},
			{MessageID: "msg_assistant", SessionKey: claimed.Event.SessionKey, SessionID: claimed.Event.ActiveSessionID, RunID: claimed.RunID, Role: "assistant", Content: "done", Visible: true, CreatedAt: now},
		},
		Now: now,
	}); err != nil {
		t.Fatalf("finalize run: %v", err)
	}

	history, err := db.RecentMessages(ctx, claimed.Event.ActiveSessionID, 10)
	if err != nil {
		t.Fatalf("recent messages: %v", err)
	}
	if len(history) != 4 {
		t.Fatalf("expected 4 history messages, got %+v", history)
	}
	if history[1].Role != "assistant" || len(history[1].ToolCalls) != 1 {
		t.Fatalf("expected assistant tool_calls restored, got %+v", history)
	}
	if history[1].ToolCalls[0].ToolCallID != "call_1" || history[1].ToolCalls[0].Name != "memory_search" {
		t.Fatalf("unexpected restored tool calls: %+v", history[1].ToolCalls)
	}
	if history[2].Role != "tool" || history[2].ToolCallID != "call_1" {
		t.Fatalf("expected tool result after assistant tool_calls, got %+v", history)
	}
}

func TestFinalizeRunRecentMessagesRestoresMeta(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	now := time.Now().UTC()
	result, err := db.IngestEvent(ctx, "local", "local:dm:u1", persistRequest(api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv:cron:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "cron_fire", Text: "nightly tick"},
	}), "sha256:test-cron", now)
	if err != nil {
		t.Fatalf("ingest event: %v", err)
	}
	if err := db.MarkEventQueued(ctx, result.EventID, now); err != nil {
		t.Fatalf("mark queued: %v", err)
	}
	claimed, ok, err := db.ClaimEvent(ctx, result.EventID, "run_cron_1", now)
	if err != nil || !ok {
		t.Fatalf("claim event ok=%v err=%v", ok, err)
	}
	if err := db.FinalizeRun(ctx, RunFinalize{
		RunID:       claimed.RunID,
		EventID:     claimed.Event.EventID,
		SessionKey:  claimed.Event.SessionKey,
		SessionID:   claimed.Event.ActiveSessionID,
		RunMode:     model.RunModeNoReply,
		RunStatus:   model.RunStatusCompleted,
		EventStatus: model.EventStatusSuppressed,
		Messages: []StoredMessage{{
			MessageID:  "msg_cron_user",
			SessionKey: claimed.Event.SessionKey,
			SessionID:  claimed.Event.ActiveSessionID,
			RunID:      claimed.RunID,
			Role:       "user",
			Content:    "nightly tick",
			Visible:    false,
			Meta:       map[string]any{"payload_type": "cron_fire"},
			CreatedAt:  now,
		}},
		Now: now,
	}); err != nil {
		t.Fatalf("finalize run: %v", err)
	}

	history, err := db.RecentMessages(ctx, claimed.Event.ActiveSessionID, 10)
	if err != nil {
		t.Fatalf("recent messages: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history message, got %+v", history)
	}
	if history[0].Meta["payload_type"] != "cron_fire" {
		t.Fatalf("expected payload_type meta restored, got %+v", history[0])
	}
}

func TestRecentMessagesForPromptSkipsCronFireWithoutShrinkingWindow(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	now := time.Now().UTC()
	sessionKey := "local:dm:u1"
	activeSessionID := ""

	seedRun := func(index int, at time.Time, payloadType string, text string, messages []StoredMessage) {
		result, err := db.IngestEvent(ctx, "local", sessionKey, persistRequest(api.IngestRequest{
			Source:         "cli",
			Conversation:   model.Conversation{ConversationID: "conv", ChannelType: "dm", ParticipantID: "u1"},
			IdempotencyKey: fmt.Sprintf("cli:conv:%d", index),
			Timestamp:      at.Format(time.RFC3339Nano),
			Payload:        model.EventPayload{Type: payloadType, Text: text},
		}), fmt.Sprintf("sha256:test:%d", index), at)
		if err != nil {
			t.Fatalf("ingest event %d: %v", index, err)
		}
		if err := db.MarkEventQueued(ctx, result.EventID, at); err != nil {
			t.Fatalf("mark queued %d: %v", index, err)
		}
		claimed, ok, err := db.ClaimEvent(ctx, result.EventID, fmt.Sprintf("run_%d", index), at)
		if err != nil || !ok {
			t.Fatalf("claim event %d ok=%v err=%v", index, ok, err)
		}
		if activeSessionID == "" {
			activeSessionID = claimed.Event.ActiveSessionID
		}
		for i := range messages {
			messages[i].SessionKey = claimed.Event.SessionKey
			messages[i].SessionID = claimed.Event.ActiveSessionID
			messages[i].RunID = claimed.RunID
			messages[i].CreatedAt = at
		}
		if err := db.FinalizeRun(ctx, RunFinalize{
			RunID:       claimed.RunID,
			EventID:     claimed.Event.EventID,
			SessionKey:  claimed.Event.SessionKey,
			SessionID:   claimed.Event.ActiveSessionID,
			RunMode:     runModeForPayloadType(payloadType),
			RunStatus:   model.RunStatusCompleted,
			EventStatus: eventStatusForPayloadType(payloadType),
			Messages:    messages,
			Now:         at,
		}); err != nil {
			t.Fatalf("finalize run %d: %v", index, err)
		}
	}

	seedRun(1, now, "message", "hello", []StoredMessage{
		{MessageID: "msg_user", Role: "user", Content: "hello", Visible: true},
		{MessageID: "msg_assistant", Role: "assistant", Content: "world", Visible: true},
	})
	for i := 0; i < 3; i++ {
		seedRun(
			i+2,
			now.Add(time.Duration(i+1)*time.Second),
			"cron_fire",
			"nightly tick",
			[]StoredMessage{{
				MessageID: fmt.Sprintf("msg_cron_%d", i),
				Role:      "user",
				Content:   "nightly tick",
				Visible:   false,
				Meta:      map[string]any{"payload_type": "cron_fire"},
			}},
		)
	}

	history, err := db.RecentMessagesForPrompt(ctx, activeSessionID, 2)
	if err != nil {
		t.Fatalf("recent prompt messages: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 prompt history messages, got %+v", history)
	}
	if history[0].Role != "user" || history[0].Content != "hello" {
		t.Fatalf("expected oldest prompt history user message, got %+v", history)
	}
	if history[1].Role != "assistant" || history[1].Content != "world" {
		t.Fatalf("expected oldest prompt history assistant message, got %+v", history)
	}
}

func TestRecentMessagesForPromptSkipsNewSessionWithoutShrinkingWindow(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	now := time.Now().UTC()
	sessionKey := "local:dm:u1"
	activeSessionID := ""

	seedRun := func(index int, at time.Time, payloadType string, text string, messages []StoredMessage) {
		result, err := db.IngestEvent(ctx, "local", sessionKey, persistRequest(api.IngestRequest{
			Source:         "cli",
			Conversation:   model.Conversation{ConversationID: "conv", ChannelType: "dm", ParticipantID: "u1"},
			IdempotencyKey: fmt.Sprintf("cli:new:%d", index),
			Timestamp:      at.Format(time.RFC3339Nano),
			Payload:        model.EventPayload{Type: payloadType, Text: text},
		}), fmt.Sprintf("sha256:new:%d", index), at)
		if err != nil {
			t.Fatalf("ingest event %d: %v", index, err)
		}
		if err := db.MarkEventQueued(ctx, result.EventID, at); err != nil {
			t.Fatalf("mark queued %d: %v", index, err)
		}
		claimed, ok, err := db.ClaimEvent(ctx, result.EventID, fmt.Sprintf("run_new_%d", index), at)
		if err != nil || !ok {
			t.Fatalf("claim event %d ok=%v err=%v", index, ok, err)
		}
		if activeSessionID == "" {
			activeSessionID = claimed.Event.ActiveSessionID
		}
		for i := range messages {
			messages[i].SessionKey = claimed.Event.SessionKey
			messages[i].SessionID = claimed.Event.ActiveSessionID
			messages[i].RunID = claimed.RunID
			messages[i].CreatedAt = at
		}
		if err := db.FinalizeRun(ctx, RunFinalize{
			RunID:       claimed.RunID,
			EventID:     claimed.Event.EventID,
			SessionKey:  claimed.Event.SessionKey,
			SessionID:   claimed.Event.ActiveSessionID,
			RunMode:     runModeForPayloadType(payloadType),
			RunStatus:   model.RunStatusCompleted,
			EventStatus: eventStatusForPayloadType(payloadType),
			Messages:    messages,
			Now:         at,
		}); err != nil {
			t.Fatalf("finalize run %d: %v", index, err)
		}
	}

	seedRun(1, now, "new_session", "/new", []StoredMessage{
		{MessageID: "msg_new_user", Role: "user", Content: "/new", Visible: true, Meta: map[string]any{"payload_type": "new_session"}},
		{MessageID: "msg_new_assistant", Role: "assistant", Content: "已开始新会话。", Visible: true, Meta: map[string]any{"payload_type": "new_session"}},
	})
	seedRun(2, now.Add(time.Second), "message", "hello", []StoredMessage{
		{MessageID: "msg_user", Role: "user", Content: "hello", Visible: true},
		{MessageID: "msg_assistant", Role: "assistant", Content: "world", Visible: true},
	})

	history, err := db.RecentMessagesForPrompt(ctx, activeSessionID, 2)
	if err != nil {
		t.Fatalf("recent prompt messages: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 prompt history messages, got %+v", history)
	}
	if history[0].Role != "user" || history[0].Content != "hello" {
		t.Fatalf("expected user message to remain, got %+v", history)
	}
	if history[1].Role != "assistant" || history[1].Content != "world" {
		t.Fatalf("expected assistant message to remain, got %+v", history)
	}
}

func TestConversationDMScopeRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	conv := model.Conversation{ConversationID: "conv_scope", ChannelType: "dm", ParticipantID: "u1"}
	now := time.Now().UTC()

	if _, ok, err := db.GetConversationDMScope(ctx, "local", conv); err != nil || ok {
		t.Fatalf("expected empty scope before insert, ok=%v err=%v", ok, err)
	}
	if err := db.WithWriterTx(ctx, func(tx *sql.Tx) error {
		return upsertConversationDMScopeTx(ctx, tx, "local", conv, "scope_abc", now)
	}); err != nil {
		t.Fatalf("upsertConversationDMScopeTx: %v", err)
	}
	scope, ok, err := db.GetConversationDMScope(ctx, "local", conv)
	if err != nil || !ok {
		t.Fatalf("GetConversationDMScope ok=%v err=%v", ok, err)
	}
	if scope != "scope_abc" {
		t.Fatalf("expected scope_abc, got %q", scope)
	}
}

func runModeForPayloadType(payloadType string) model.RunMode {
	if payloadType == "cron_fire" || payloadType == "memory_flush" || payloadType == "compaction" {
		return model.RunModeNoReply
	}
	return model.RunModeNormal
}

func eventStatusForPayloadType(payloadType string) model.EventStatus {
	if payloadType == "cron_fire" || payloadType == "memory_flush" || payloadType == "compaction" {
		return model.EventStatusSuppressed
	}
	return model.EventStatusProcessed
}

func TestRecoverExpiredProcessingAndOutboxLease(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	now := time.Now().UTC()
	result, err := db.IngestEvent(ctx, "local", "local:dm:u1", persistRequest(api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv", ChannelType: "dm", ParticipantID: "u1"},
		IdempotencyKey: "cli:conv:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello retry"},
	}), "sha256:test", now)
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

func TestOpenAddsOutboxRoutingColumns(t *testing.T) {
	workspace := t.TempDir()
	path := DBPath(workspace)
	db, err := openSQLite(path, DefaultBusyTimeout())
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE outbox (
			outbox_id TEXT PRIMARY KEY,
			event_id TEXT NOT NULL,
			session_key TEXT NOT NULL,
			body TEXT NOT NULL,
			status TEXT NOT NULL,
			next_attempt_at TEXT NOT NULL,
			locked_at TEXT NOT NULL DEFAULT '',
			lock_owner TEXT NOT NULL DEFAULT '',
			attempt_count INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			sent_at TEXT NOT NULL DEFAULT ''
		);
		PRAGMA user_version = 1;
	`); err != nil {
		t.Fatalf("seed old schema: %v", err)
	}
	_ = db.Close()

	opened, err := Open(workspace, DefaultBusyTimeout())
	if err != nil {
		t.Fatalf("open db with migration: %v", err)
	}
	defer opened.Close()

	for _, column := range []string{"channel", "target_id"} {
		exists, err := tableColumnExists(opened.writer, "outbox", column)
		if err != nil {
			t.Fatalf("tableColumnExists(%s): %v", column, err)
		}
		if !exists {
			t.Fatalf("expected outbox.%s to exist after migration", column)
		}
	}
	if !tablePresent(t, opened.writer, "conversation_scopes") {
		t.Fatalf("expected conversation_scopes to exist after migration")
	}
}

func tablePresent(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		t.Fatalf("query sqlite_master for %s: %v", table, err)
	}
	return name == table
}
