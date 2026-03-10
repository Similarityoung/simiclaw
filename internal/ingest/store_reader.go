package ingest

import (
	"context"

	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type storeSessionReader struct {
	db *store.DB
}

func NewStoreSessionReader(db *store.DB) SessionReader {
	return storeSessionReader{db: db}
}

func (r storeSessionReader) GetConversationDMScope(ctx context.Context, tenantID string, conv model.Conversation) (string, bool, error) {
	return r.db.GetConversationDMScope(ctx, tenantID, conv)
}

func (r storeSessionReader) GetScopeSession(ctx context.Context, sessionKey string) (SessionScopeRecord, bool, error) {
	rec, ok, err := r.db.GetSession(ctx, sessionKey)
	if err != nil || !ok {
		return SessionScopeRecord{}, ok, err
	}
	return SessionScopeRecord{
		ConversationID: rec.ConversationID,
		ChannelType:    rec.ChannelType,
		ParticipantID:  rec.ParticipantID,
		DMScope:        rec.DMScope,
	}, true, nil
}
