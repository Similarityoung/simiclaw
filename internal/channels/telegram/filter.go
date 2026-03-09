package telegram

import (
	"strings"

	tele "gopkg.in/telebot.v4"
)

func isAllowedPrivateTextMessage(botUserID int64, allowedUserIDs map[int64]struct{}, msg *tele.Message) (bool, string) {
	if msg == nil {
		return false, "missing_message"
	}
	if msg.Chat == nil {
		return false, "missing_chat"
	}
	if msg.Chat.Type != tele.ChatPrivate {
		return false, "non_private_chat"
	}
	if msg.Sender == nil {
		return false, "missing_sender"
	}
	if msg.Sender.IsBot {
		return false, "bot_sender"
	}
	if botUserID != 0 && msg.Sender.ID == botUserID {
		return false, "self_sender"
	}
	if strings.TrimSpace(msg.Text) == "" {
		return false, "empty_text"
	}
	if len(allowedUserIDs) == 0 {
		return false, "user_not_allowed"
	}
	if _, ok := allowedUserIDs[msg.Sender.ID]; !ok {
		return false, "user_not_allowed"
	}
	return true, ""
}
