package telegram

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	Date      int64  `json:"date"`
	Text      string `json:"text,omitempty"`
	Chat      Chat   `json:"chat"`
	From      User   `json:"from"`
}

type CallbackQuery struct {
	ID      string  `json:"id"`
	Data    string  `json:"data"`
	From    User    `json:"from"`
	Message Message `json:"message"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type User struct {
	ID int64 `json:"id"`
}

func Normalize(update Update) (model.IngestRequest, error) {
	if update.UpdateID == 0 {
		return model.IngestRequest{}, fmt.Errorf("update_id is required")
	}
	var conv model.Conversation
	var payload model.EventPayload
	var ts time.Time

	switch {
	case update.Message != nil:
		conv = model.Conversation{
			ConversationID: fmt.Sprintf("tg_chat_%d", update.Message.Chat.ID),
			ChannelType:    mapChatType(update.Message.Chat.Type),
			ParticipantID:  fmt.Sprintf("%d", update.Message.From.ID),
		}
		payload = model.EventPayload{Type: "message", Text: update.Message.Text}
		ts = time.Unix(update.Message.Date, 0).UTC()
	case update.CallbackQuery != nil:
		conv = model.Conversation{
			ConversationID: fmt.Sprintf("tg_chat_%d", update.CallbackQuery.Message.Chat.ID),
			ChannelType:    mapChatType(update.CallbackQuery.Message.Chat.Type),
			ParticipantID:  fmt.Sprintf("%d", update.CallbackQuery.From.ID),
		}
		payload = model.EventPayload{Type: "button", Text: update.CallbackQuery.Data}
		ts = time.Now().UTC()
	default:
		return model.IngestRequest{}, fmt.Errorf("unsupported telegram update")
	}

	native, _ := json.Marshal(update)
	payload.Native = native

	return model.IngestRequest{
		Source:         "telegram",
		Conversation:   conv,
		IdempotencyKey: fmt.Sprintf("telegram:update:%d", update.UpdateID),
		Timestamp:      ts.Format(time.RFC3339),
		Payload:        payload,
	}, nil
}

func mapChatType(v string) string {
	if v == "private" {
		return "dm"
	}
	if v == "group" || v == "supergroup" {
		return "group"
	}
	return "group"
}
