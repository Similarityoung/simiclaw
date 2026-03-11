package ingest

import (
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

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
