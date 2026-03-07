package cli

import (
	"fmt"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func BuildIngestRequest(conversationID, participantID string, seq int64, text string) model.IngestRequest {
	if conversationID == "" {
		conversationID = "cli_default"
	}
	if participantID == "" {
		participantID = "local_user"
	}
	return model.IngestRequest{
		Source: "cli",
		Conversation: model.Conversation{
			ConversationID: conversationID,
			ChannelType:    "dm",
			ParticipantID:  participantID,
		},
		IdempotencyKey: fmt.Sprintf("cli:%s:%d", conversationID, seq),
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Payload: model.EventPayload{
			Type: "message",
			Text: text,
		},
	}
}
