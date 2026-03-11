package telegram

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/api"

	"github.com/similarityyoung/simiclaw/pkg/model"
	tele "gopkg.in/telebot.v4"
)

func NormalizeTextUpdate(update tele.Update, receivedAt time.Time) (api.IngestRequest, error) {
	if update.ID == 0 {
		return api.IngestRequest{}, fmt.Errorf("update_id is required")
	}
	if update.Message == nil {
		return api.IngestRequest{}, fmt.Errorf("telegram text update requires message")
	}
	msg := update.Message
	if msg.Chat == nil {
		return api.IngestRequest{}, fmt.Errorf("telegram message chat is required")
	}
	if msg.Sender == nil {
		return api.IngestRequest{}, fmt.Errorf("telegram message sender is required")
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return api.IngestRequest{}, fmt.Errorf("telegram message text is required")
	}
	native, err := json.Marshal(update)
	if err != nil {
		return api.IngestRequest{}, err
	}
	receivedAt = receivedAt.UTC()
	return api.IngestRequest{
		Source: "telegram",
		Conversation: model.Conversation{
			ConversationID: fmt.Sprintf("tg_chat_%d", msg.Chat.ID),
			ChannelType:    "dm",
			ParticipantID:  fmt.Sprintf("%d", msg.Sender.ID),
		},
		IdempotencyKey: fmt.Sprintf("telegram:update:%d", update.ID),
		Timestamp:      receivedAt.Format(time.RFC3339Nano),
		Payload: model.EventPayload{
			Type:   "message",
			Text:   text,
			Native: native,
			Extra: map[string]string{
				"telegram_chat_id":        fmt.Sprintf("%d", msg.Chat.ID),
				"telegram_message_id":     fmt.Sprintf("%d", msg.ID),
				"telegram_update_id":      fmt.Sprintf("%d", update.ID),
				"telegram_participant_id": fmt.Sprintf("%d", msg.Sender.ID),
			},
		},
	}, nil
}
