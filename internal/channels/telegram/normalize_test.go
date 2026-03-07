package telegram

import "testing"

func TestNormalizeMessage(t *testing.T) {
	req, err := Normalize(Update{
		UpdateID: 123,
		Message: &Message{
			Date: 1700000000,
			Text: "hello",
			Chat: Chat{ID: 999, Type: "private"},
			From: User{ID: 1001},
		},
	})
	if err != nil {
		t.Fatalf("Normalize error: %v", err)
	}
	if req.Source != "telegram" {
		t.Fatalf("unexpected source: %s", req.Source)
	}
	if req.IdempotencyKey != "telegram:update:123" {
		t.Fatalf("unexpected key: %s", req.IdempotencyKey)
	}
	if req.Conversation.ChannelType != "dm" {
		t.Fatalf("expected dm, got %s", req.Conversation.ChannelType)
	}
}
