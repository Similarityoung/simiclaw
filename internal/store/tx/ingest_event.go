package tx

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	"github.com/similarityyoung/simiclaw/internal/gateway/bindings"
	storeprojections "github.com/similarityyoung/simiclaw/internal/store/projections"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (r *RuntimeRepository) PersistEvent(ctx context.Context, tenantID, sessionKey string, req gateway.PersistRequest, payloadHash string, now time.Time) (gateway.PersistResult, error) {
	var result gateway.PersistResult
	err := r.db.WithWriterTx(ctx, func(tx *sql.Tx) error {
		var (
			existingEventID    string
			existingHash       string
			existingSessionKey string
			existingSessionID  string
			createdAt          string
		)
		err := tx.QueryRowContext(
			ctx,
			`SELECT event_id, payload_hash, session_key, session_id, created_at FROM idempotency_keys WHERE key = ?`,
			req.IdempotencyKey,
		).Scan(&existingEventID, &existingHash, &existingSessionKey, &existingSessionID, &createdAt)
		if err == nil {
			if existingHash != payloadHash {
				return gateway.ErrIdempotencyConflict
			}
			result = gateway.PersistResult{
				EventID:         existingEventID,
				SessionKey:      existingSessionKey,
				SessionID:       existingSessionID,
				ReceivedAt:      mustParseTime(createdAt),
				PayloadHash:     existingHash,
				Duplicate:       true,
				ExistingEventID: existingEventID,
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		dmScope := bindings.NormalizeScope(req.DMScope)
		if req.DMScope != "" {
			if err := upsertConversationDMScopeTx(ctx, tx, tenantID, req.Conversation, dmScope, now); err != nil {
				return err
			}
		}

		sessionID, err := storeprojections.ResolveSessionTx(ctx, tx, sessionKey, req.Conversation, dmScope, now)
		if err != nil {
			return err
		}

		eventID := fmt.Sprintf("evt_%d", now.UnixNano())
		payloadJSON, err := json.Marshal(req.Payload)
		if err != nil {
			return err
		}
		nowText := timeText(now)
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO idempotency_keys (key, event_id, payload_hash, session_key, session_id, created_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			req.IdempotencyKey,
			eventID,
			payloadHash,
			sessionKey,
			sessionID,
			nowText,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO events (
				event_id, source, tenant_id, conversation_id, channel_type, participant_id,
				session_key, session_id, idempotency_key, payload_type, payload_text,
				payload_json, payload_hash, status, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			eventID,
			req.Source,
			tenantID,
			req.Conversation.ConversationID,
			req.Conversation.ChannelType,
			req.Conversation.ParticipantID,
			sessionKey,
			sessionID,
			req.IdempotencyKey,
			req.Payload.Type,
			req.Payload.Text,
			string(payloadJSON),
			payloadHash,
			string(model.EventStatusReceived),
			nowText,
			nowText,
		); err != nil {
			return err
		}

		result = gateway.PersistResult{
			EventID:     eventID,
			SessionKey:  sessionKey,
			SessionID:   sessionID,
			ReceivedAt:  now,
			PayloadHash: payloadHash,
		}
		return nil
	})
	return result, err
}

func (r *RuntimeRepository) MarkEventQueued(ctx context.Context, eventID string, now time.Time) error {
	_, err := r.db.Writer().ExecContext(
		ctx,
		`UPDATE events SET status = ?, updated_at = ? WHERE event_id = ? AND status = ?`,
		string(model.EventStatusQueued),
		timeText(now),
		eventID,
		string(model.EventStatusReceived),
	)
	return err
}

func (r *RuntimeRepository) GetConversationDMScope(ctx context.Context, tenantID string, conv model.Conversation) (string, bool, error) {
	return r.db.GetConversationDMScope(ctx, tenantID, conv)
}

func upsertConversationDMScopeTx(ctx context.Context, tx *sql.Tx, tenantID string, conv model.Conversation, dmScope string, now time.Time) error {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO conversation_scopes (
			tenant_id, conversation_id, channel_type, participant_id, dm_scope, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(tenant_id, conversation_id, channel_type, participant_id)
		DO UPDATE SET dm_scope = excluded.dm_scope, updated_at = excluded.updated_at`,
		tenantID,
		conv.ConversationID,
		conv.ChannelType,
		conversationParticipantID(conv),
		bindings.NormalizeScope(dmScope),
		timeText(now),
	)
	return err
}

func conversationParticipantID(conv model.Conversation) string {
	if conv.ChannelType == "dm" {
		return conv.ParticipantID
	}
	return ""
}
