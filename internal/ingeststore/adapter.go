package ingeststore

import (
	"context"
	"time"

	"github.com/similarityyoung/simiclaw/internal/ingest/port"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type Adapter struct {
	db *store.DB
}

func New(db *store.DB) *Adapter {
	return &Adapter{db: db}
}

func (a *Adapter) PersistEvent(ctx context.Context, tenantID, sessionKey string, req port.PersistRequest, payloadHash string, now time.Time) (port.PersistResult, error) {
	return a.db.IngestEvent(ctx, tenantID, sessionKey, req, payloadHash, now)
}

func (a *Adapter) MarkEventQueued(ctx context.Context, eventID string, now time.Time) error {
	return a.db.MarkEventQueued(ctx, eventID, now)
}

func (a *Adapter) GetConversationDMScope(ctx context.Context, tenantID string, conv model.Conversation) (string, bool, error) {
	return a.db.GetConversationDMScope(ctx, tenantID, conv)
}

func (a *Adapter) GetScopeSession(ctx context.Context, sessionKey string) (port.SessionScopeRecord, bool, error) {
	rec, ok, err := a.db.GetSession(ctx, sessionKey)
	if err != nil || !ok {
		return port.SessionScopeRecord{}, ok, err
	}
	return port.SessionScopeRecord{
		ConversationID: rec.ConversationID,
		ChannelType:    rec.ChannelType,
		ParticipantID:  rec.ParticipantID,
		DMScope:        rec.DMScope,
	}, true, nil
}
