package ingeststore

import (
	"context"
	"errors"
	"time"

	"github.com/similarityyoung/simiclaw/internal/ingest"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type Adapter struct {
	db *store.DB
}

func New(db *store.DB) *Adapter {
	return &Adapter{db: db}
}

func (a *Adapter) PersistEvent(ctx context.Context, tenantID, sessionKey string, req ingest.PersistRequest, payloadHash string, now time.Time) (ingest.PersistResult, error) {
	result, err := a.db.IngestEvent(ctx, tenantID, sessionKey, api.IngestRequest{
		Source:         req.Source,
		Conversation:   req.Conversation,
		Payload:        req.Payload,
		IdempotencyKey: req.IdempotencyKey,
		DMScope:        req.DMScope,
	}, payloadHash, now)
	if err != nil {
		if errors.Is(err, store.ErrIdempotencyConflict) {
			return ingest.PersistResult{}, ingest.ErrIdempotencyConflict
		}
		return ingest.PersistResult{}, err
	}
	return ingest.PersistResult{
		EventID:         result.EventID,
		SessionKey:      result.SessionKey,
		SessionID:       result.SessionID,
		ReceivedAt:      result.ReceivedAt,
		PayloadHash:     result.PayloadHash,
		Duplicate:       result.Duplicate,
		ExistingEventID: result.ExistingEventID,
	}, nil
}

func (a *Adapter) MarkEventQueued(ctx context.Context, eventID string, now time.Time) error {
	return a.db.MarkEventQueued(ctx, eventID, now)
}

func (a *Adapter) GetConversationDMScope(ctx context.Context, tenantID string, conv model.Conversation) (string, bool, error) {
	return a.db.GetConversationDMScope(ctx, tenantID, conv)
}

func (a *Adapter) GetScopeSession(ctx context.Context, sessionKey string) (ingest.SessionScopeRecord, bool, error) {
	rec, ok, err := a.db.GetSession(ctx, sessionKey)
	if err != nil || !ok {
		return ingest.SessionScopeRecord{}, ok, err
	}
	return ingest.SessionScopeRecord{
		ConversationID: rec.ConversationID,
		ChannelType:    rec.ChannelType,
		ParticipantID:  rec.ParticipantID,
		DMScope:        rec.DMScope,
	}, true, nil
}
