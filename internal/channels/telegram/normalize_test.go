package telegram

import (
	"testing"
	"time"

	tele "gopkg.in/telebot.v4"
)

func TestNormalizeTextUpdate(t *testing.T) {
	receivedAt := time.Date(2026, 3, 9, 10, 11, 12, 123000000, time.UTC)
	req, err := NormalizeTextUpdate(tele.Update{
		ID: 123,
		Message: &tele.Message{
			ID:       456,
			Unixtime: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC).Unix(),
			Text:     "hello",
			Chat:     &tele.Chat{ID: 999, Type: tele.ChatPrivate},
			Sender:   &tele.User{ID: 1001},
		},
	}, receivedAt)
	if err != nil {
		t.Fatalf("NormalizeTextUpdate error: %v", err)
	}
	if req.Source != "telegram" {
		t.Fatalf("unexpected source: %s", req.Source)
	}
	if req.IdempotencyKey != "telegram:update:123" {
		t.Fatalf("unexpected key: %s", req.IdempotencyKey)
	}
	if !req.Timestamp.Equal(receivedAt) {
		t.Fatalf("expected received_at timestamp, got %s", req.Timestamp.Format(time.RFC3339Nano))
	}
	if req.Conversation.ConversationID != "tg_chat_999" {
		t.Fatalf("unexpected conversation id: %s", req.Conversation.ConversationID)
	}
	if req.Conversation.ChannelType != "dm" {
		t.Fatalf("expected dm, got %s", req.Conversation.ChannelType)
	}
	if req.Conversation.ParticipantID != "1001" {
		t.Fatalf("unexpected participant id: %s", req.Conversation.ParticipantID)
	}
	if req.Payload.Type != "message" || req.Payload.Text != "hello" {
		t.Fatalf("unexpected payload: %+v", req.Payload)
	}
	if req.Payload.Extra["telegram_chat_id"] != "999" {
		t.Fatalf("unexpected chat id extra: %+v", req.Payload.Extra)
	}
	if req.Payload.Extra["telegram_message_id"] != "456" {
		t.Fatalf("unexpected message id extra: %+v", req.Payload.Extra)
	}
	if req.Payload.Extra["telegram_update_id"] != "123" {
		t.Fatalf("unexpected update id extra: %+v", req.Payload.Extra)
	}
	if req.Payload.Extra["telegram_participant_id"] != "1001" {
		t.Fatalf("unexpected participant id extra: %+v", req.Payload.Extra)
	}
}
