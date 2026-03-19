package cli

import (
	"fmt"
	"time"

	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func NormalizeMessage(conversationID, participantID string, seq int64, text string, now time.Time) gatewaymodel.NormalizedIngress {
	if conversationID == "" {
		conversationID = "cli_default"
	}
	if participantID == "" {
		participantID = "local_user"
	}
	return gatewaymodel.NormalizedIngress{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: conversationID,
			ChannelType:    "dm",
			ParticipantID:  participantID,
		},
		IdempotencyKey: fmt.Sprintf("cli:%s:%d", conversationID, seq),
		Timestamp:      now.UTC(),
		Payload: model.EventPayload{
			Type: "message",
			Text: text,
		},
	}
}
