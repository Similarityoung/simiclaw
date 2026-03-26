package runtime

import (
	"fmt"
	"strconv"
	"strings"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type deliveryIntentResolver struct{}

func (deliveryIntentResolver) Resolve(event model.InternalEvent, reply string) (*runtimemodel.DeliveryIntent, error) {
	if strings.TrimSpace(reply) == "" {
		return nil, nil
	}
	intent := &runtimemodel.DeliveryIntent{Body: reply}
	if event.Source != "telegram" {
		return intent, nil
	}
	chatID, err := telegramTargetID(event)
	if err != nil {
		return nil, err
	}
	intent.Channel = "telegram"
	intent.TargetID = chatID
	return intent, nil
}

func telegramTargetID(event model.InternalEvent) (string, error) {
	if event.Payload.Extra == nil {
		return "", fmt.Errorf("telegram event missing payload.extra.telegram_chat_id")
	}
	raw := strings.TrimSpace(event.Payload.Extra["telegram_chat_id"])
	if raw == "" {
		return "", fmt.Errorf("telegram event missing payload.extra.telegram_chat_id")
	}
	chatID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid payload.extra.telegram_chat_id %q: %w", raw, err)
	}
	return strconv.FormatInt(chatID, 10), nil
}
