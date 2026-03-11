package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/similarityyoung/simiclaw/internal/ingest/port"
	"github.com/similarityyoung/simiclaw/internal/readmodel"
	sessionpkg "github.com/similarityyoung/simiclaw/internal/session"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

var nilSentinel = errors.New("no-op")

func (db *DB) IngestEvent(ctx context.Context, tenantID, sessionKey string, req port.PersistRequest, payloadHash string, now time.Time) (port.PersistResult, error) {
	var result port.PersistResult
	err := db.WithWriterTx(ctx, func(tx *sql.Tx) error {
		var existing readmodel.LookupEvent
		var createdAt string
		err := tx.QueryRowContext(
			ctx,
			`SELECT event_id, payload_hash, session_key, session_id, created_at FROM idempotency_keys WHERE key = ?`,
			req.IdempotencyKey,
		).Scan(&existing.EventID, &existing.PayloadHash, &existing.SessionKey, &existing.SessionID, &createdAt)
		if err == nil {
			if existing.PayloadHash != payloadHash {
				return port.ErrIdempotencyConflict
			}
			existing.ReceivedAt = mustParseTime(createdAt)
			result = port.PersistResult{
				EventID:         existing.EventID,
				SessionKey:      existing.SessionKey,
				SessionID:       existing.SessionID,
				ReceivedAt:      existing.ReceivedAt,
				PayloadHash:     existing.PayloadHash,
				Duplicate:       true,
				ExistingEventID: existing.EventID,
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		dmScope := sessionpkg.NormalizeScope(req.DMScope)
		if req.DMScope != "" {
			if err := upsertConversationDMScopeTx(ctx, tx, tenantID, req.Conversation, dmScope, now); err != nil {
				return err
			}
		}

		sessionID, err := resolveSessionTx(ctx, tx, sessionKey, req.Conversation, dmScope, now)
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
		result = port.PersistResult{
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

func (db *DB) MarkEventQueued(ctx context.Context, eventID string, now time.Time) error {
	_, err := db.writer.ExecContext(
		ctx,
		`UPDATE events SET status = ?, updated_at = ? WHERE event_id = ? AND status = ?`,
		string(model.EventStatusQueued),
		timeText(now),
		eventID,
		string(model.EventStatusReceived),
	)
	return err
}

func (db *DB) LookupInbound(ctx context.Context, key string) (readmodel.LookupEvent, bool, error) {
	var row readmodel.LookupEvent
	var createdAt string
	err := db.reader.QueryRowContext(
		ctx,
		`SELECT event_id, payload_hash, session_key, session_id, created_at FROM idempotency_keys WHERE key = ?`,
		key,
	).Scan(&row.EventID, &row.PayloadHash, &row.SessionKey, &row.SessionID, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return readmodel.LookupEvent{}, false, nil
	}
	if err != nil {
		return readmodel.LookupEvent{}, false, err
	}
	row.ReceivedAt = mustParseTime(createdAt)
	return row, true, nil
}

func (db *DB) GetEvent(ctx context.Context, eventID string) (readmodel.EventRecord, bool, error) {
	rows, err := db.reader.QueryContext(ctx, eventSelectSQL+` WHERE event_id = ?`, eventID)
	if err != nil {
		return readmodel.EventRecord{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return readmodel.EventRecord{}, false, rows.Err()
	}
	rec, err := scanEvent(rows)
	if err != nil {
		return readmodel.EventRecord{}, false, err
	}
	return rec, true, rows.Err()
}

func (db *DB) ListRunnableEventIDs(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 64
	}
	rows, err := db.reader.QueryContext(
		ctx,
		`SELECT event_id FROM events WHERE status IN (?, ?) ORDER BY updated_at ASC, event_id ASC LIMIT ?`,
		string(model.EventStatusReceived),
		string(model.EventStatusQueued),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (db *DB) ClaimEvent(ctx context.Context, eventID, runID string, now time.Time) (ClaimedEvent, bool, error) {
	var claimed ClaimedEvent
	err := db.WithWriterTx(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(
			ctx,
			`UPDATE events
			 SET status = ?, processing_started_at = ?, updated_at = ?, run_id = ?, run_mode = ?
			 WHERE event_id = ?
			   AND status IN (?, ?)
			   AND NOT EXISTS (
			       SELECT 1 FROM runs
			       WHERE runs.event_id = events.event_id AND runs.status = ?
			   )
			 RETURNING source, tenant_id, conversation_id, channel_type, participant_id, session_key, session_id, idempotency_key, payload_json, payload_type, created_at`,
			string(model.EventStatusProcessing),
			timeText(now),
			timeText(now),
			runID,
			string(model.RunModeNormal),
			eventID,
			string(model.EventStatusReceived),
			string(model.EventStatusQueued),
			string(model.RunStatusStarted),
		)

		var (
			source         string
			tenantID       string
			conversationID string
			channelType    string
			participantID  string
			sessionKey     string
			sessionID      string
			idempotencyKey string
			payloadJSON    string
			payloadType    string
			createdAt      string
		)
		if err := row.Scan(
			&source,
			&tenantID,
			&conversationID,
			&channelType,
			&participantID,
			&sessionKey,
			&sessionID,
			&idempotencyKey,
			&payloadJSON,
			&payloadType,
			&createdAt,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nilSentinel
			}
			return err
		}

		var payload model.EventPayload
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return err
		}
		runMode := model.RunModeNormal
		if isNoReplyPayload(payload.Type) {
			runMode = model.RunModeNoReply
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO runs (run_id, event_id, session_key, session_id, run_mode, status, started_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			runID,
			eventID,
			sessionKey,
			sessionID,
			string(runMode),
			string(model.RunStatusStarted),
			timeText(now),
		); err != nil {
			return err
		}
		claimed = ClaimedEvent{
			Event: model.InternalEvent{
				EventID:         eventID,
				Source:          source,
				TenantID:        tenantID,
				Conversation:    model.Conversation{ConversationID: conversationID, ChannelType: channelType, ParticipantID: participantID},
				SessionKey:      sessionKey,
				IdempotencyKey:  idempotencyKey,
				Timestamp:       mustParseTime(createdAt),
				Payload:         payload,
				ActiveSessionID: sessionID,
			},
			RunID:   runID,
			Status:  model.EventStatusProcessing,
			RunMode: runMode,
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE events SET run_mode = ? WHERE event_id = ?`,
			string(runMode),
			eventID,
		); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, nilSentinel) {
			return ClaimedEvent{}, false, nil
		}
		return ClaimedEvent{}, false, err
	}
	if claimed.Event.EventID == "" {
		return ClaimedEvent{}, false, nil
	}
	return claimed, true, nil
}

func (db *DB) RecoverExpiredProcessing(ctx context.Context, cutoff, now time.Time) ([]string, error) {
	rows, err := db.reader.QueryContext(
		ctx,
		`SELECT event_id FROM events
		 WHERE status = ? AND COALESCE(NULLIF(processing_started_at, ''), updated_at) <= ?
		 ORDER BY updated_at ASC, event_id ASC`,
		string(model.EventStatusProcessing),
		timeText(cutoff),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if err := db.WithWriterTx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE runs
				 SET status = ?, error_code = ?, error_message = ?, finished_at = ?
				 WHERE event_id = ? AND status = ?`,
				string(model.RunStatusFailed),
				model.ErrorCodeInternal,
				"processing lease expired",
				timeText(now),
				id,
				string(model.RunStatusStarted),
			); err != nil {
				return err
			}
			_, err := tx.ExecContext(
				ctx,
				`UPDATE events
				 SET status = ?, processing_started_at = '', updated_at = ?, error_code = ?, error_message = ?
				 WHERE event_id = ? AND status = ?`,
				string(model.EventStatusQueued),
				timeText(now),
				model.ErrorCodeInternal,
				"processing lease expired",
				id,
				string(model.EventStatusProcessing),
			)
			return err
		}); err != nil {
			return ids, err
		}
	}
	return ids, nil
}

func scanEvent(rows *sql.Rows) (readmodel.EventRecord, error) {
	var (
		rec          readmodel.EventRecord
		createdAt    string
		receivedAt   string
		updatedAt    string
		errorCode    string
		errorMessage string
	)
	if err := rows.Scan(
		&rec.EventID,
		&rec.Status,
		&rec.OutboxStatus,
		&rec.SessionKey,
		&rec.SessionID,
		&rec.RunID,
		&rec.RunMode,
		&rec.AssistantReply,
		&rec.OutboxID,
		&rec.ProcessingLease,
		&rec.PayloadHash,
		&rec.Provider,
		&rec.Model,
		&rec.ProviderRequestID,
		&createdAt,
		&receivedAt,
		&updatedAt,
		&errorCode,
		&errorMessage,
	); err != nil {
		return readmodel.EventRecord{}, err
	}
	rec.CreatedAt = mustParseTime(createdAt)
	rec.ReceivedAt = mustParseTime(receivedAt)
	rec.UpdatedAt = mustParseTime(updatedAt)
	if errorCode != "" || errorMessage != "" {
		rec.Error = &model.ErrorBlock{Code: errorCode, Message: errorMessage}
	}
	return rec, nil
}

const eventSelectSQL = `
SELECT event_id, status, outbox_status, session_key, session_id, run_id, run_mode, assistant_reply,
       outbox_id, processing_started_at, payload_hash, provider, model, provider_request_id,
       created_at, created_at, updated_at, error_code, error_message
FROM events`
