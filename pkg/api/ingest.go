package api

import "github.com/similarityyoung/simiclaw/pkg/model"

type IngestRequest struct {
	Source         string             `json:"source"`
	Conversation   model.Conversation `json:"conversation"`
	DMScope        string             `json:"dm_scope,omitempty"`
	SessionKeyHint string             `json:"session_key,omitempty"`
	IdempotencyKey string             `json:"idempotency_key"`
	Timestamp      string             `json:"timestamp"`
	Payload        model.EventPayload `json:"payload"`
}

type IngestResponse struct {
	EventID         string            `json:"event_id"`
	SessionKey      string            `json:"session_key"`
	ActiveSessionID string            `json:"active_session_id"`
	ReceivedAt      string            `json:"received_at"`
	PayloadHash     string            `json:"payload_hash"`
	Status          string            `json:"status"`
	StatusURL       string            `json:"status_url"`
	Error           *model.ErrorBlock `json:"error,omitempty"`
}
