package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestIngestEventDuplicateConflictAndConversationScope(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	now := time.Now().UTC()

	req := api.IngestRequest{
		Source: "telegram",
		Conversation: model.Conversation{
			ConversationID: "scope-test",
			ChannelType:    "dm",
			ParticipantID:  "u1",
		},
		DMScope:        " scoped-room ",
		IdempotencyKey: "telegram:scope-test:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload: model.EventPayload{
			Type: "message",
			Text: "hello scope",
		},
	}

	first, err := db.IngestEvent(ctx, "local", "local:dm:u1", persistRequest(req), "sha256:dup", now)
	if err != nil {
		t.Fatalf("first IngestEvent: %v", err)
	}
	duplicate, err := db.IngestEvent(ctx, "local", "local:dm:u1:changed", persistRequest(req), "sha256:dup", now.Add(time.Second))
	if err != nil {
		t.Fatalf("duplicate IngestEvent: %v", err)
	}
	if !duplicate.Duplicate || duplicate.EventID != first.EventID || duplicate.ExistingEventID != first.EventID {
		t.Fatalf("expected duplicate ingest to reuse first event, first=%+v duplicate=%+v", first, duplicate)
	}
	if duplicate.SessionKey != first.SessionKey || duplicate.SessionID != first.SessionID {
		t.Fatalf("expected duplicate ingest to return stored session binding, first=%+v duplicate=%+v", first, duplicate)
	}

	scope, ok, err := db.GetConversationDMScope(ctx, "local", req.Conversation)
	if err != nil || !ok {
		t.Fatalf("GetConversationDMScope ok=%v err=%v", ok, err)
	}
	if scope != "scoped-room" {
		t.Fatalf("expected normalized scope, got %q", scope)
	}

	_, err = db.IngestEvent(ctx, "local", "local:dm:u1", persistRequest(req), "sha256:conflict", now.Add(2*time.Second))
	if !errors.Is(err, gateway.ErrIdempotencyConflict) {
		t.Fatalf("expected gateway.ErrIdempotencyConflict, got %v", err)
	}
}

func TestScheduledJobClaimMissAndDefaultCronInterval(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	now := time.Now().UTC()

	if _, ok, err := db.ClaimScheduledJob(ctx, model.ScheduledJobKindCron, "worker-a", now); err != nil || ok {
		t.Fatalf("expected no runnable cron job, ok=%v err=%v", ok, err)
	}

	if _, err := db.Writer().ExecContext(
		ctx,
		`INSERT INTO scheduled_jobs (
			job_id, name, kind, status, payload_json, next_run_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"cron:default-interval",
		"default-interval",
		string(model.ScheduledJobKindCron),
		string(model.ScheduledJobStatusActive),
		`{"source":"cron","tenant_id":"local","conversation":{"conversation_id":"conv","channel_type":"dm","participant_id":"u1"},"payload":{"type":"cron_fire","text":"tick"}}`,
		timeText(now),
		timeText(now),
		timeText(now),
	); err != nil {
		t.Fatalf("insert scheduled job: %v", err)
	}

	job := ClaimedJob{
		JobID:   "cron:default-interval",
		Kind:    model.ScheduledJobKindCron,
		Status:  model.ScheduledJobStatusActive,
		Payload: ScheduledJobPayload{},
	}
	if err := db.CompleteScheduledJob(ctx, job, now); err != nil {
		t.Fatalf("CompleteScheduledJob: %v", err)
	}

	var (
		status    string
		nextRunAt string
	)
	if err := db.Reader().QueryRowContext(
		ctx,
		`SELECT status, next_run_at FROM scheduled_jobs WHERE job_id = ?`,
		job.JobID,
	).Scan(&status, &nextRunAt); err != nil {
		t.Fatalf("query scheduled job: %v", err)
	}
	if status != string(model.ScheduledJobStatusActive) {
		t.Fatalf("expected cron job to stay active, got %q", status)
	}

	next := mustParseTime(nextRunAt)
	if delta := next.Sub(now); delta < 9*time.Second || delta > 11*time.Second {
		t.Fatalf("expected default cron interval near 10s, got %v", delta)
	}
}

func TestMissingLookupsReturnFalse(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	if _, ok, err := db.LookupInbound(ctx, "missing"); err != nil || ok {
		t.Fatalf("LookupInbound expected missing result, ok=%v err=%v", ok, err)
	}
	if _, ok, err := db.GetEvent(ctx, "missing"); err != nil || ok {
		t.Fatalf("GetEvent expected missing result, ok=%v err=%v", ok, err)
	}
	if _, ok, err := db.GetRun(ctx, "missing"); err != nil || ok {
		t.Fatalf("GetRun expected missing result, ok=%v err=%v", ok, err)
	}
	if _, ok, err := db.GetSession(ctx, "missing"); err != nil || ok {
		t.Fatalf("GetSession expected missing result, ok=%v err=%v", ok, err)
	}
}

func TestListMessagesCursorFallbackAndToolPayloadDecode(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	now := time.Now().UTC()

	req := api.IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: "messages-cursor",
			ChannelType:    "dm",
			ParticipantID:  "u1",
		},
		IdempotencyKey: "cli:messages-cursor:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload: model.EventPayload{
			Type: "message",
			Text: "hello cursor",
		},
	}
	result, err := db.IngestEvent(ctx, "local", "local:dm:u1", persistRequest(req), "sha256:messages-cursor", now)
	if err != nil {
		t.Fatalf("IngestEvent: %v", err)
	}
	if err := db.MarkEventQueued(ctx, result.EventID, now); err != nil {
		t.Fatalf("MarkEventQueued: %v", err)
	}
	claimed, ok, err := db.ClaimEvent(ctx, result.EventID, "run_messages_cursor", now)
	if err != nil || !ok {
		t.Fatalf("ClaimEvent ok=%v err=%v", ok, err)
	}
	if err := db.FinalizeRun(ctx, RunFinalize{
		RunID:          claimed.RunID,
		EventID:        claimed.Event.EventID,
		SessionKey:     claimed.Event.SessionKey,
		SessionID:      claimed.Event.ActiveSessionID,
		RunMode:        model.RunModeNormal,
		RunStatus:      model.RunStatusCompleted,
		EventStatus:    model.EventStatusProcessed,
		AssistantReply: "done",
		Messages: []StoredMessage{
			{MessageID: "msg_cursor_user", SessionKey: claimed.Event.SessionKey, SessionID: claimed.Event.ActiveSessionID, RunID: claimed.RunID, Role: "user", Content: "hello cursor", Visible: true, CreatedAt: now},
			{
				MessageID:  "msg_cursor_tool",
				SessionKey: claimed.Event.SessionKey,
				SessionID:  claimed.Event.ActiveSessionID,
				RunID:      claimed.RunID,
				Role:       "tool",
				Content:    "tool result",
				Visible:    true,
				ToolCallID: "call_cursor",
				ToolName:   "memory_search",
				ToolArgs:   map[string]any{"query": "cursor"},
				ToolResult: map[string]any{"hits": 1},
				Meta:       map[string]any{"source": "test"},
				CreatedAt:  now.Add(time.Millisecond),
			},
			{MessageID: "msg_cursor_assistant", SessionKey: claimed.Event.SessionKey, SessionID: claimed.Event.ActiveSessionID, RunID: claimed.RunID, Role: "assistant", Content: "done", Visible: true, CreatedAt: now.Add(2 * time.Millisecond)},
		},
		Now: now.Add(2 * time.Millisecond),
	}); err != nil {
		t.Fatalf("FinalizeRun: %v", err)
	}

	items, err := db.ListMessages(ctx, claimed.Event.ActiveSessionID, 10, time.Time{}, "msg_cursor_assistant", false)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected all messages with cursor fallback, got %+v", items)
	}

	tool := items[1]
	if tool.ToolCallID != "call_cursor" || tool.ToolName != "memory_search" {
		t.Fatalf("expected decoded tool identity, got %+v", tool)
	}
	if query, _ := tool.ToolArgs["query"].(string); query != "cursor" {
		t.Fatalf("expected decoded tool args, got %+v", tool.ToolArgs)
	}
	hits, ok := tool.ToolResult["hits"].(float64)
	if !ok || hits != 1 {
		t.Fatalf("expected decoded tool result, got %+v", tool.ToolResult)
	}
	if tool.Meta["source"] != "test" {
		t.Fatalf("expected decoded meta, got %+v", tool.Meta)
	}
}

func TestHelperDefaults(t *testing.T) {
	if got := normalizeListLimit(0); got != 50 {
		t.Fatalf("expected default list limit 50, got %d", got)
	}
	if got := normalizeListLimit(7); got != 7 {
		t.Fatalf("expected explicit list limit, got %d", got)
	}
	if got := conversationParticipantID(model.Conversation{ChannelType: "dm", ParticipantID: "u1"}); got != "u1" {
		t.Fatalf("expected dm participant to be preserved, got %q", got)
	}
	if got := conversationParticipantID(model.Conversation{ChannelType: "group", ParticipantID: "u1"}); got != "" {
		t.Fatalf("expected non-dm participant to be blank, got %q", got)
	}
}
