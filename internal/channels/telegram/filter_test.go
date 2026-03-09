package telegram

import (
	"testing"

	tele "gopkg.in/telebot.v4"
)

func TestIsAllowedPrivateTextMessage(t *testing.T) {
	allowed := map[int64]struct{}{1001: {}}
	msg := &tele.Message{
		Text:   "hello",
		Chat:   &tele.Chat{ID: 42, Type: tele.ChatPrivate},
		Sender: &tele.User{ID: 1001},
	}
	ok, reason := isAllowedPrivateTextMessage(9999, allowed, msg)
	if !ok || reason != "" {
		t.Fatalf("expected allowed message, got ok=%v reason=%q", ok, reason)
	}
}

func TestIsAllowedPrivateTextMessageRejectsNotAllowedUser(t *testing.T) {
	msg := &tele.Message{
		Text:   "hello",
		Chat:   &tele.Chat{ID: 42, Type: tele.ChatPrivate},
		Sender: &tele.User{ID: 2002},
	}
	ok, reason := isAllowedPrivateTextMessage(9999, map[int64]struct{}{1001: {}}, msg)
	if ok || reason != "user_not_allowed" {
		t.Fatalf("expected user_not_allowed, got ok=%v reason=%q", ok, reason)
	}
}

func TestIsAllowedPrivateTextMessageRejectsBotAndSelf(t *testing.T) {
	allowed := map[int64]struct{}{1001: {}, 9999: {}}
	botMsg := &tele.Message{
		Text:   "hello",
		Chat:   &tele.Chat{ID: 42, Type: tele.ChatPrivate},
		Sender: &tele.User{ID: 1001, IsBot: true},
	}
	if ok, reason := isAllowedPrivateTextMessage(9999, allowed, botMsg); ok || reason != "bot_sender" {
		t.Fatalf("expected bot_sender, got ok=%v reason=%q", ok, reason)
	}
	selfMsg := &tele.Message{
		Text:   "hello",
		Chat:   &tele.Chat{ID: 42, Type: tele.ChatPrivate},
		Sender: &tele.User{ID: 9999},
	}
	if ok, reason := isAllowedPrivateTextMessage(9999, allowed, selfMsg); ok || reason != "self_sender" {
		t.Fatalf("expected self_sender, got ok=%v reason=%q", ok, reason)
	}
}

func TestIsAllowedPrivateTextMessageRejectsNonPrivate(t *testing.T) {
	msg := &tele.Message{
		Text:   "hello",
		Chat:   &tele.Chat{ID: 42, Type: tele.ChatGroup},
		Sender: &tele.User{ID: 1001},
	}
	ok, reason := isAllowedPrivateTextMessage(9999, map[int64]struct{}{1001: {}}, msg)
	if ok || reason != "non_private_chat" {
		t.Fatalf("expected non_private_chat, got ok=%v reason=%q", ok, reason)
	}
}
