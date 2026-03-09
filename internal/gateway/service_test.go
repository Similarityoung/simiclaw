package gateway

import (
	"context"
	"testing"
	"time"

	sessionpkg "github.com/similarityyoung/simiclaw/internal/session"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestValidateRequestRejectsBadNativeRef(t *testing.T) {
	now := time.Now().UTC()
	_, err := validateRequest(model.IngestRequest{
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

func TestValidateRequestIdempotencyFormat(t *testing.T) {
	now := time.Now().UTC()
	_, err := validateRequest(model.IngestRequest{
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
	req := model.IngestRequest{
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
	req := model.IngestRequest{
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

func TestResolveRequestScopePrefersSessionKeyHint(t *testing.T) {
	ctx := context.Background()
	db := newGatewayTestDB(t)
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

	svc := &Service{cfg: config.Default(), db: db}
	req, scope, apiErr := svc.resolveRequestScope(ctx, model.IngestRequest{
		Source:         "web",
		Conversation:   conv,
		SessionKeyHint: oldSessionKey,
		IdempotencyKey: "web:conv_1:1",
		Timestamp:      now,
		Payload:        model.EventPayload{Type: "message", Text: "back to old"},
	})
	if apiErr != nil {
		t.Fatalf("resolve request scope apiErr=%+v", apiErr)
	}
	if scope != "scope_old" || req.DMScope != "scope_old" {
		t.Fatalf("expected session hint scope_old, got scope=%q req=%+v", scope, req)
	}
}

func TestSessionRateLimitKeyIgnoresDMScope(t *testing.T) {
	reqA := model.IngestRequest{
		Conversation: model.Conversation{ConversationID: "conv_1", ChannelType: "dm", ParticipantID: "u1"},
		DMScope:      "scope_old",
	}
	reqB := model.IngestRequest{
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

func newGatewayTestDB(t *testing.T) *store.DB {
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
