package outbound

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

type RouterSender struct {
	defaultSender Sender
	telegram      TelegramTextSender
}

func NewRouterSender(defaultSender Sender, telegram TelegramTextSender) RouterSender {
	return RouterSender{defaultSender: defaultSender, telegram: telegram}
}

func (s RouterSender) Send(ctx context.Context, msg model.OutboxMessage) error {
	switch strings.TrimSpace(msg.Channel) {
	case "", "stdout":
		return s.defaultSender.Send(ctx, msg)
	case "telegram":
		if s.telegram == nil {
			return fmt.Errorf("telegram sender not configured")
		}
		chatID, err := strconv.ParseInt(strings.TrimSpace(msg.TargetID), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid telegram target_id %q: %w", msg.TargetID, err)
		}
		return s.telegram.SendTextMessage(ctx, chatID, msg.Body)
	default:
		return fmt.Errorf("unsupported outbox channel %q", msg.Channel)
	}
}
