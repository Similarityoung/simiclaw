package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	sessionpkg "github.com/similarityyoung/simiclaw/internal/session"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type captureQueue struct {
	eventIDs []string
}

type testDBAdapter struct {
	db *store.DB
}

func (a testDBAdapter) PersistEvent(ctx context.Context, tenantID, sessionKey string, req PersistRequest, payloadHash string, now time.Time) (PersistResult, error) {
	result, err := a.db.IngestEvent(ctx, tenantID, sessionKey, api.IngestRequest{
		Source:         req.Source,
		Conversation:   req.Conversation,
		Payload:        req.Payload,
		IdempotencyKey: req.IdempotencyKey,
		DMScope:        req.DMScope,
	}, payloadHash, now)
	if err != nil {
		if errors.Is(err, store.ErrIdempotencyConflict) {
			return PersistResult{}, ErrIdempotencyConflict
		}
		return PersistResult{}, err
	}
	return PersistResult{
		EventID:         result.EventID,
		SessionKey:      result.SessionKey,
		SessionID:       result.SessionID,
		ReceivedAt:      result.ReceivedAt,
		PayloadHash:     result.PayloadHash,
		Duplicate:       result.Duplicate,
		ExistingEventID: result.ExistingEventID,
	}, nil
}

func (a testDBAdapter) MarkEventQueued(ctx context.Context, eventID string, now time.Time) error {
	return a.db.MarkEventQueued(ctx, eventID, now)
}

func (a testDBAdapter) GetConversationDMScope(ctx context.Context, tenantID string, conv model.Conversation) (string, bool, error) {
	return a.db.GetConversationDMScope(ctx, tenantID, conv)
}

func (a testDBAdapter) GetScopeSession(ctx context.Context, sessionKey string) (SessionScopeRecord, bool, error) {
	rec, ok, err := a.db.GetSession(ctx, sessionKey)
	if err != nil || !ok {
		return SessionScopeRecord{}, ok, err
	}
	return SessionScopeRecord{
		ConversationID: rec.ConversationID,
		ChannelType:    rec.ChannelType,
		ParticipantID:  rec.ParticipantID,
		DMScope:        rec.DMScope,
	}, true, nil
}

func (q *captureQueue) TryEnqueue(eventID string) bool {
	q.eventIDs = append(q.eventIDs, eventID)
	return true
}

func TestValidateRequestRejectsBadNativeRef(t *testing.T) {
	now := time.Now().UTC()
	_, err := validateRequest(api.IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: "conv_1",
			ChannelType:    "dm",
			ParticipantID:  "u1",
		},
		IdempotencyKey: "cli:conv_1:1",
		Timestamp:      now.Format(time.RFC3339),
		Payload: model.EventPayload{
			Type:      "message",
			Text:      "hi",
			NativeRef: "../../etc/passwd",
		},
	}, now)
	if err == nil {
		t.Fatalf("expected invalid native_ref error")
	}
}

func TestValidateRequestAllowsWindowsNativeRefSeparators(t *testing.T) {
	now := time.Now().UTC()
	_, err := validateRequest(api.IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: "conv_1",
			ChannelType:    "dm",
			ParticipantID:  "u1",
		},
		IdempotencyKey: "cli:conv_1:1",
		Timestamp:      now.Format(time.RFC3339),
		Payload: model.EventPayload{
			Type:      "message",
			Text:      "hi",
			NativeRef: `runtime\native\events\evt_1.json`,
		},
	}, now)
	if err != nil {
		t.Fatalf("expected windows-style native_ref to pass validation, got %v", err)
	}
}

func TestValidateRequestRejectsWindowsNativeRefTraversal(t *testing.T) {
	now := time.Now().UTC()
	_, err := validateRequest(api.IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: "conv_1",
			ChannelType:    "dm",
			ParticipantID:  "u1",
		},
		IdempotencyKey: "cli:conv_1:1",
		Timestamp:      now.Format(time.RFC3339),
		Payload: model.EventPayload{
			Type:      "message",
			Text:      "hi",
			NativeRef: `..\runtime\native\evt_1.json`,
		},
	}, now)
	if err == nil {
		t.Fatalf("expected windows traversal native_ref to be rejected")
	}
}

func TestValidateRequestIdempotencyFormat(t *testing.T) {
	now := time.Now().UTC()
	_, err := validateRequest(api.IngestRequest{
		Source: "telegram",
		Conversation: model.Conversation{
			ConversationID: "conv_1",
			ChannelType:    "dm",
			ParticipantID:  "u1",
		},
		IdempotencyKey: "telegram:update:abc",
		Timestamp:      now.Format(time.RFC3339),
		Payload:        model.EventPayload{Type: "message", Text: "hi"},
	}, now)
	if err == nil {
		t.Fatalf("expected idempotency format error")
	}
}

func TestCanonicalPayloadHashStable(t *testing.T) {
	req := api.IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: "conv_1",
			ChannelType:    "dm",
			ParticipantID:  "u1",
		},
		IdempotencyKey: "cli:conv_1:1",
		Timestamp:      "2026-03-03T12:00:00Z",
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}
	h1, err := canonicalPayloadHash(req)
	if err != nil {
		t.Fatalf("hash error: %v", err)
	}
	h2, err := canonicalPayloadHash(req)
	if err != nil {
		t.Fatalf("hash error: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("hash not stable: %s vs %s", h1, h2)
	}
}

func TestCanonicalPayloadHashIgnoresDMScope(t *testing.T) {
	req := api.IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: "conv_1",
			ChannelType:    "dm",
			ParticipantID:  "u1",
		},
		IdempotencyKey: "cli:conv_1:1",
		Timestamp:      "2026-03-03T12:00:00Z",
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}
	base, err := canonicalPayloadHash(req)
	if err != nil {
		t.Fatalf("hash error: %v", err)
	}
	req.DMScope = "scope_123"
	withScope, err := canonicalPayloadHash(req)
	if err != nil {
		t.Fatalf("hash error: %v", err)
	}
	if base != withScope {
		t.Fatalf("expected dm_scope to be ignored, got %s vs %s", base, withScope)
	}
}

func TestScopeResolverPrefersSessionKeyHint(t *testing.T) {
	ctx := context.Background()
	db := newIngestTestDB(t)
	conv := model.Conversation{ConversationID: "conv_1", ChannelType: "dm", ParticipantID: "u1"}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	oldSessionKey, err := sessionpkg.ComputeKey("local", conv, "scope_old")
	if err != nil {
		t.Fatalf("compute old session key: %v", err)
	}

	if _, err := db.Writer().ExecContext(
		ctx,
		`INSERT INTO sessions (
			session_key, active_session_id, conversation_id, channel_type, participant_id, dm_scope,
			last_activity_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		oldSessionKey,
		"ses_old",
		conv.ConversationID,
		conv.ChannelType,
		conv.ParticipantID,
		"scope_old",
		now,
		now,
		now,
	); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := db.Writer().ExecContext(
		ctx,
		`INSERT INTO conversation_scopes (tenant_id, conversation_id, channel_type, participant_id, dm_scope, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"local",
		conv.ConversationID,
		conv.ChannelType,
		conv.ParticipantID,
		"scope_new",
		now,
	); err != nil {
		t.Fatalf("insert conversation scope: %v", err)
	}

	resolver := NewScopeResolver("local", testDBAdapter{db: db})
	req, scope, ingestErr := resolver.Resolve(ctx, api.IngestRequest{
		Source:         "web",
		Conversation:   conv,
		SessionKeyHint: oldSessionKey,
		IdempotencyKey: "web:conv_1:1",
		Timestamp:      now,
		Payload:        model.EventPayload{Type: "message", Text: "back to old"},
	})
	if ingestErr != nil {
		t.Fatalf("resolve request scope err=%+v", ingestErr)
	}
	if scope != "scope_old" || req.DMScope != "scope_old" {
		t.Fatalf("expected session hint scope_old, got scope=%q req=%+v", scope, req)
	}
}

func TestSessionRateLimitKeyIgnoresDMScope(t *testing.T) {
	reqA := api.IngestRequest{
		Conversation: model.Conversation{ConversationID: "conv_1", ChannelType: "dm", ParticipantID: "u1"},
		DMScope:      "scope_old",
	}
	reqB := api.IngestRequest{
		Conversation: model.Conversation{ConversationID: "conv_1", ChannelType: "dm", ParticipantID: "u1"},
		DMScope:      "scope_new",
	}

	keyA, err := sessionRateLimitKey("local", reqA)
	if err != nil {
		t.Fatalf("sessionRateLimitKey A: %v", err)
	}
	keyB, err := sessionRateLimitKey("local", reqB)
	if err != nil {
		t.Fatalf("sessionRateLimitKey B: %v", err)
	}
	if keyA != keyB {
		t.Fatalf("expected dm_scope-insensitive rate limit key, got %q vs %q", keyA, keyB)
	}
}

func TestServiceDuplicateSameScopeSamePayload(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 0, 0, 123, time.UTC)
	db := newIngestTestDB(t)
	queue := &captureQueue{}
	svc := newIngestService(t, db, queue)

	req := api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "dup", ChannelType: "dm", ParticipantID: "u1"},
		DMScope:        "scope_a",
		IdempotencyKey: "cli:dup:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}

	first, err := svc.Ingest(context.Background(), Command{Request: req, ReceivedAt: now})
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	second, err := svc.Ingest(context.Background(), Command{Request: req, ReceivedAt: now.Add(time.Second)})
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if first.Duplicate {
		t.Fatalf("first ingest should not be duplicate")
	}
	if !second.Duplicate {
		t.Fatalf("expected duplicate result, got %+v", second)
	}
	if first.EventID != second.EventID || len(queue.eventIDs) != 1 {
		t.Fatalf("expected one persisted event and one enqueue, first=%+v second=%+v queue=%+v", first, second, queue.eventIDs)
	}
}

func TestServiceDifferentScopesStayIsolated(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 5, 0, 123, time.UTC)
	db := newIngestTestDB(t)
	queue := &captureQueue{}
	svc := newIngestService(t, db, queue)

	reqA := api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "scope", ChannelType: "dm", ParticipantID: "u1"},
		DMScope:        "scope_a",
		IdempotencyKey: "cli:scope:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "same"},
	}
	reqB := reqA
	reqB.DMScope = "scope_b"
	reqB.IdempotencyKey = "cli:scope:2"
	reqB.Timestamp = now.Add(time.Second).Format(time.RFC3339Nano)

	first, err := svc.Ingest(context.Background(), Command{Request: reqA, ReceivedAt: now})
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	second, err := svc.Ingest(context.Background(), Command{Request: reqB, ReceivedAt: now.Add(time.Second)})
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if first.SessionKey == second.SessionKey {
		t.Fatalf("expected distinct session keys for different scopes, got %q", first.SessionKey)
	}
	if first.EventID == second.EventID {
		t.Fatalf("expected distinct event ids for different scopes")
	}
}

func TestServiceSameIdempotencyKeyDifferentPayloadConflicts(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 10, 0, 123, time.UTC)
	db := newIngestTestDB(t)
	svc := newIngestService(t, db, &captureQueue{})

	req := api.IngestRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conflict", ChannelType: "dm", ParticipantID: "u1"},
		DMScope:        "scope_a",
		IdempotencyKey: "cli:conflict:1",
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
	}
	if _, err := svc.Ingest(context.Background(), Command{Request: req, ReceivedAt: now}); err != nil {
		t.Fatalf("first ingest: %v", err)
	}

	req.Timestamp = now.Add(time.Second).Format(time.RFC3339Nano)
	req.Payload.Text = "changed"
	if _, err := svc.Ingest(context.Background(), Command{Request: req, ReceivedAt: now.Add(time.Second)}); err == nil || err.Code != model.ErrorCodeConflict {
		t.Fatalf("expected conflict error, got %+v", err)
	}
}

func TestServiceInheritsConversationScopeWithoutManualDMScope(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 15, 0, 123, time.UTC)
	db := newIngestTestDB(t)
	svc := newIngestService(t, db, &captureQueue{})
	conv := model.Conversation{ConversationID: "inherit", ChannelType: "dm", ParticipantID: "u1"}
	if _, err := db.Writer().ExecContext(
		context.Background(),
		`INSERT INTO conversation_scopes (tenant_id, conversation_id, channel_type, participant_id, dm_scope, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"local",
		conv.ConversationID,
		conv.ChannelType,
		conv.ParticipantID,
		"scope_saved",
		now.Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("insert conversation scope: %v", err)
	}

	result, err := svc.Ingest(context.Background(), Command{
		Request: api.IngestRequest{
			Source:         "cli",
			Conversation:   conv,
			IdempotencyKey: "cli:inherit:1",
			Timestamp:      now.Format(time.RFC3339Nano),
			Payload:        model.EventPayload{Type: "message", Text: "hello"},
		},
		ReceivedAt: now,
	})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}

	wantSessionKey, computeErr := sessionpkg.ComputeKey("local", conv, "scope_saved")
	if computeErr != nil {
		t.Fatalf("compute session key: %v", computeErr)
	}
	if result.SessionKey != wantSessionKey {
		t.Fatalf("expected inherited session key %q, got %q", wantSessionKey, result.SessionKey)
	}
}

func newIngestService(t *testing.T, db *store.DB, queue *captureQueue) *Service {
	t.Helper()
	adapter := testDBAdapter{db: db}
	return NewService("local", adapter, queue, NewScopeResolver("local", adapter), 100, 100, 100, 100)
}

func newIngestTestDB(t *testing.T) *store.DB {
	t.Helper()
	workspace := t.TempDir()
	if err := store.InitWorkspace(workspace, false, store.DefaultBusyTimeout()); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	db, err := store.Open(workspace, store.DefaultBusyTimeout())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}
