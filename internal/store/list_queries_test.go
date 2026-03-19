package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestListEventsPageUsesSQLiteFilteringAndCursor(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

	insertEventRow(t, db, "evt_a", "local:dm:u1", "ses_1", model.EventStatusProcessed, base, base.Add(10*time.Minute))
	insertEventRow(t, db, "evt_b", "local:dm:u1", "ses_1", model.EventStatusProcessed, base.Add(time.Minute), base.Add(2*time.Hour))
	insertEventRow(t, db, "evt_c", "local:dm:u1", "ses_1", model.EventStatusProcessed, base.Add(2*time.Minute), base.Add(2*time.Minute))
	insertEventRow(t, db, "evt_other_status", "local:dm:u1", "ses_1", model.EventStatusQueued, base.Add(3*time.Minute), base.Add(3*time.Minute))
	insertEventRow(t, db, "evt_other_session", "local:dm:u2", "ses_2", model.EventStatusProcessed, base.Add(4*time.Minute), base.Add(4*time.Minute))

	page1, err := db.ListEventsPage(ctx, EventListFilter{
		SessionKey: "local:dm:u1",
		Status:     model.EventStatusProcessed,
		Limit:      2,
	})
	if err != nil {
		t.Fatalf("ListEventsPage page1: %v", err)
	}
	if len(page1) != 2 || page1[0].EventID != "evt_c" || page1[1].EventID != "evt_b" {
		t.Fatalf("unexpected first page: %+v", page1)
	}

	page2, err := db.ListEventsPage(ctx, EventListFilter{
		SessionKey:      "local:dm:u1",
		Status:          model.EventStatusProcessed,
		Limit:           2,
		CursorCreatedAt: page1[1].CreatedAt,
		CursorEventID:   page1[1].EventID,
	})
	if err != nil {
		t.Fatalf("ListEventsPage page2: %v", err)
	}
	if len(page2) != 1 || page2[0].EventID != "evt_a" {
		t.Fatalf("expected cursor to advance by created_at/event_id, got %+v", page2)
	}
}

func TestListRunsPageFiltersAndBoundaryCursor(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	base := time.Date(2026, 3, 10, 13, 0, 0, 0, time.UTC)

	insertRunRow(t, db, "run_a", "evt_a", "local:dm:u1", "ses_1", base)
	insertRunRow(t, db, "run_b", "evt_b", "local:dm:u1", "ses_1", base.Add(time.Minute))
	insertRunRow(t, db, "run_c", "evt_c", "local:dm:u1", "ses_1", base.Add(2*time.Minute))
	insertRunRow(t, db, "run_other", "evt_d", "local:dm:u2", "ses_2", base.Add(3*time.Minute))

	page1, err := db.ListRunsPage(ctx, RunListFilter{SessionKey: "local:dm:u1", Limit: 2})
	if err != nil {
		t.Fatalf("ListRunsPage page1: %v", err)
	}
	if len(page1) != 2 || page1[0].RunID != "run_c" || page1[1].RunID != "run_b" {
		t.Fatalf("unexpected first page: %+v", page1)
	}

	page2, err := db.ListRunsPage(ctx, RunListFilter{
		SessionKey:      "local:dm:u1",
		Limit:           2,
		CursorStartedAt: page1[1].StartedAt,
		CursorRunID:     page1[1].RunID,
	})
	if err != nil {
		t.Fatalf("ListRunsPage page2: %v", err)
	}
	if len(page2) != 1 || page2[0].RunID != "run_a" {
		t.Fatalf("unexpected second page: %+v", page2)
	}

	empty, err := db.ListRunsPage(ctx, RunListFilter{
		SessionKey:      "local:dm:u1",
		Limit:           2,
		CursorStartedAt: page2[0].StartedAt,
		CursorRunID:     page2[0].RunID,
	})
	if err != nil {
		t.Fatalf("ListRunsPage empty: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty page after boundary cursor, got %+v", empty)
	}
}

func TestListSessionsPageFiltersAndCursor(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	base := time.Date(2026, 3, 10, 14, 0, 0, 0, time.UTC)

	insertSessionRow(t, db, "local:dm:u1:scope_a", "ses_1", "conv_1", base)
	insertSessionRow(t, db, "local:dm:u1:scope_b", "ses_2", "conv_1", base.Add(time.Minute))
	insertSessionRow(t, db, "local:dm:u1:scope_c", "ses_3", "conv_1", base.Add(2*time.Minute))
	insertSessionRow(t, db, "local:dm:u2:scope_a", "ses_4", "conv_2", base.Add(3*time.Minute))

	page1, err := db.ListSessionsPage(ctx, SessionListFilter{
		ConversationID: "conv_1",
		Limit:          2,
	})
	if err != nil {
		t.Fatalf("ListSessionsPage page1: %v", err)
	}
	if len(page1) != 2 || page1[0].SessionKey != "local:dm:u1:scope_c" || page1[1].SessionKey != "local:dm:u1:scope_b" {
		t.Fatalf("unexpected first page: %+v", page1)
	}

	page2, err := db.ListSessionsPage(ctx, SessionListFilter{
		ConversationID:       "conv_1",
		Limit:                2,
		CursorLastActivityAt: page1[1].LastActivityAt,
		CursorLastSessionKey: page1[1].SessionKey,
	})
	if err != nil {
		t.Fatalf("ListSessionsPage page2: %v", err)
	}
	if len(page2) != 1 || page2[0].SessionKey != "local:dm:u1:scope_a" {
		t.Fatalf("unexpected second page: %+v", page2)
	}
}

func insertEventRow(t *testing.T, db *runtimeDB, eventID, sessionKey, sessionID string, status model.EventStatus, createdAt, updatedAt time.Time) {
	t.Helper()
	if _, err := db.Writer().ExecContext(
		context.Background(),
		`INSERT INTO events (
			event_id, source, tenant_id, conversation_id, channel_type, participant_id,
			session_key, session_id, idempotency_key, payload_type, payload_text,
			payload_json, payload_hash, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		eventID,
		"cli",
		"local",
		"conv",
		"dm",
		"u1",
		sessionKey,
		sessionID,
		"idem:"+eventID,
		"message",
		eventID,
		`{"type":"message","text":"`+eventID+`"}`,
		"sha256:"+eventID,
		string(status),
		timeText(createdAt),
		timeText(updatedAt),
	); err != nil {
		t.Fatalf("insert event %s: %v", eventID, err)
	}
}

func insertRunRow(t *testing.T, db *runtimeDB, runID, eventID, sessionKey, sessionID string, startedAt time.Time) {
	t.Helper()
	if _, err := db.Writer().ExecContext(
		context.Background(),
		`INSERT INTO runs (
			run_id, event_id, session_key, session_id, run_mode, status, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		runID,
		eventID,
		sessionKey,
		sessionID,
		string(model.RunModeNormal),
		string(model.RunStatusCompleted),
		timeText(startedAt),
		timeText(startedAt.Add(time.Second)),
	); err != nil {
		t.Fatalf("insert run %s: %v", runID, err)
	}
}

func insertSessionRow(t *testing.T, db *runtimeDB, sessionKey, sessionID, conversationID string, lastActivity time.Time) {
	t.Helper()
	if _, err := db.Writer().ExecContext(
		context.Background(),
		`INSERT INTO sessions (
			session_key, active_session_id, conversation_id, channel_type, participant_id, dm_scope,
			last_activity_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionKey,
		sessionID,
		conversationID,
		"dm",
		"u1",
		"default",
		timeText(lastActivity),
		timeText(lastActivity),
		timeText(lastActivity),
	); err != nil {
		t.Fatalf("insert session %s: %v", sessionKey, err)
	}
}
