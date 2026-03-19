package runtime

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

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
