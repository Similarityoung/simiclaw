package gateway

import (
	"testing"
	"time"

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
