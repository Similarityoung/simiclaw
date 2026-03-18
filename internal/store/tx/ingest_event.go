package tx

import (
	"context"
	"time"

	"github.com/similarityyoung/simiclaw/internal/ingest/port"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (r *RuntimeRepository) PersistEvent(ctx context.Context, tenantID, sessionKey string, req port.PersistRequest, payloadHash string, now time.Time) (port.PersistResult, error) {
	return r.db.IngestEvent(ctx, tenantID, sessionKey, req, payloadHash, now)
}

func (r *RuntimeRepository) MarkEventQueued(ctx context.Context, eventID string, now time.Time) error {
	return r.db.MarkEventQueued(ctx, eventID, now)
}

func (r *RuntimeRepository) GetConversationDMScope(ctx context.Context, tenantID string, conv model.Conversation) (string, bool, error) {
	return r.db.GetConversationDMScope(ctx, tenantID, conv)
}

func (r *RuntimeRepository) GetScopeSession(ctx context.Context, sessionKey string) (port.SessionScopeRecord, bool, error) {
	rec, ok, err := r.db.GetSession(ctx, sessionKey)
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
