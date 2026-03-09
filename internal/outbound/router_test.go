package outbound

import (
	"context"
	"testing"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type captureSender struct {
	msg model.OutboxMessage
	err error
}

func (s *captureSender) Send(_ context.Context, msg model.OutboxMessage) error {
	s.msg = msg
	return s.err
}

type captureTelegramSender struct {
	chatID int64
	text   string
	err    error
}

func (s *captureTelegramSender) SendTextMessage(_ context.Context, chatID int64, text string) error {
	s.chatID = chatID
	s.text = text
	return s.err
}

func TestRouterSenderRoutesTelegram(t *testing.T) {
	stdout := &captureSender{}
	telegram := &captureTelegramSender{}
	router := NewRouterSender(stdout, telegram)
	err := router.Send(context.Background(), model.OutboxMessage{Channel: "telegram", TargetID: "12345", Body: "hi"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if telegram.chatID != 12345 || telegram.text != "hi" {
		t.Fatalf("unexpected telegram send: %+v", telegram)
	}
	if stdout.msg.Body != "" {
		t.Fatalf("stdout sender should not be used: %+v", stdout.msg)
	}
}

func TestRouterSenderFallsBackToStdout(t *testing.T) {
	stdout := &captureSender{}
	router := NewRouterSender(stdout, nil)
	msg := model.OutboxMessage{Channel: "", Body: "hello stdout"}
	if err := router.Send(context.Background(), msg); err != nil {
		t.Fatalf("send: %v", err)
	}
	if stdout.msg.Body != "hello stdout" {
		t.Fatalf("unexpected stdout send: %+v", stdout.msg)
	}
}

func TestRouterSenderRejectsBadTelegramTarget(t *testing.T) {
	router := NewRouterSender(&captureSender{}, &captureTelegramSender{})
	if err := router.Send(context.Background(), model.OutboxMessage{Channel: "telegram", TargetID: "oops", Body: "hi"}); err == nil {
		t.Fatal("expected invalid target id error")
	}
}

func TestStdoutSenderStillSimulatesFailure(t *testing.T) {
	err := StdoutSender{}.Send(context.Background(), model.OutboxMessage{Body: "[fail_outbound]"})
	if err == nil {
		t.Fatal("expected simulated outbound failure")
	}
}
