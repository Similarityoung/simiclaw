package store

import (
	"github.com/similarityyoung/simiclaw/internal/ingest/port"
	"github.com/similarityyoung/simiclaw/pkg/api"
)

func persistRequest(req api.IngestRequest) port.PersistRequest {
	return port.PersistRequest{
		Source:         req.Source,
		Conversation:   req.Conversation,
		Payload:        req.Payload,
		IdempotencyKey: req.IdempotencyKey,
		DMScope:        req.DMScope,
	}
}
