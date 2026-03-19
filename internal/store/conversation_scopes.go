package store

import (
	"context"
	"database/sql"
	"errors"

	gatewaybindings "github.com/similarityyoung/simiclaw/internal/gateway/bindings"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (db *DB) GetConversationDMScope(ctx context.Context, tenantID string, conv model.Conversation) (string, bool, error) {
	participantID := conversationParticipantID(conv)
	var scope string
	err := db.reader.QueryRowContext(
		ctx,
		`SELECT dm_scope
		 FROM conversation_scopes
		 WHERE tenant_id = ? AND conversation_id = ? AND channel_type = ? AND participant_id = ?`,
		tenantID,
		conv.ConversationID,
		conv.ChannelType,
		participantID,
	).Scan(&scope)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return gatewaybindings.NormalizeScope(scope), true, nil
}

func conversationParticipantID(conv model.Conversation) string {
	if conv.ChannelType == "dm" {
		return conv.ParticipantID
	}
	return ""
}
