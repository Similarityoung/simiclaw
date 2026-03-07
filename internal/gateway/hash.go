package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func canonicalPayloadHash(req model.IngestRequest) (string, error) {
	shape := struct {
		Source         string             `json:"source"`
		Conversation   model.Conversation `json:"conversation"`
		Payload        model.EventPayload `json:"payload"`
		IdempotencyKey string             `json:"idempotency_key"`
	}{
		Source:         req.Source,
		Conversation:   req.Conversation,
		Payload:        req.Payload,
		IdempotencyKey: req.IdempotencyKey,
	}
	b, err := json.Marshal(shape)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
