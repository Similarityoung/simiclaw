package chat

import (
	"time"

	clichannel "github.com/similarityyoung/simiclaw/internal/channels/cli"
	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
	"github.com/similarityyoung/simiclaw/pkg/api"
)

func buildCLIIngestRequest(conversationID, participantID string, seq int64, text string) api.IngestRequest {
	return ingestRequestFromNormalized(clichannel.NormalizeMessage(conversationID, participantID, seq, text, time.Now().UTC()))
}

func ingestRequestFromNormalized(in gatewaymodel.NormalizedIngress) api.IngestRequest {
	return api.IngestRequest{
		Source:         in.Source,
		Conversation:   in.Conversation,
		DMScope:        in.DMScope,
		SessionKeyHint: in.SessionKeyHint,
		IdempotencyKey: in.IdempotencyKey,
		Timestamp:      in.Timestamp.UTC().Format(time.RFC3339Nano),
		Payload:        in.Payload,
	}
}
