package ingest

import (
	"context"
	"errors"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

var ErrIdempotencyConflict = errors.New("idempotency payload hash mismatch")

type PersistRequest struct {
	Source         string
	Conversation   model.Conversation
	Payload        model.EventPayload
	IdempotencyKey string
	DMScope        string
}

type PersistResult struct {
	EventID         string
	SessionKey      string
	SessionID       string
	ReceivedAt      time.Time
	PayloadHash     string
	Duplicate       bool
	ExistingEventID string
}

type Repository interface {
	PersistEvent(ctx context.Context, tenantID, sessionKey string, req PersistRequest, payloadHash string, now time.Time) (PersistResult, error)
	MarkEventQueued(ctx context.Context, eventID string, now time.Time) error
}
