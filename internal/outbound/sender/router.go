package sender

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type TelegramTextSender interface {
	SendTextMessage(ctx context.Context, chatID int64, text string) error
}

type Router struct {
	senders map[string]Sender
}

func NewRouter(defaultSender Sender, telegram TelegramTextSender) Router {
	senders := make(map[string]Sender)
	if telegram != nil {
		senders["telegram"] = telegramSender{sender: telegram}
	}
	return NewRegistry(defaultSender, senders)
}

func NewRegistry(defaultSender Sender, senders map[string]Sender) Router {
	normalized := make(map[string]Sender, len(senders))
	for channel, sender := range senders {
		if sender == nil {
			continue
		}
		normalized[strings.TrimSpace(channel)] = sender
	}
	if defaultSender != nil {
		normalized["stdout"] = defaultSender
		normalized[""] = defaultSender
	}
	return Router{senders: normalized}
}

func (r Router) Send(ctx context.Context, msg model.OutboxMessage) error {
	channel := strings.TrimSpace(msg.Channel)
	sender, ok := r.senders[channel]
	if !ok {
		return fmt.Errorf("unsupported outbox channel %q", msg.Channel)
	}
	return sender.Send(ctx, msg)
}

type telegramSender struct {
	sender TelegramTextSender
}

func (s telegramSender) Send(ctx context.Context, msg model.OutboxMessage) error {
	if s.sender == nil {
		return fmt.Errorf("telegram sender not configured")
	}
	chatID, err := strconv.ParseInt(strings.TrimSpace(msg.TargetID), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid telegram target_id %q: %w", msg.TargetID, err)
	}
	return s.sender.SendTextMessage(ctx, chatID, msg.Body)
}
