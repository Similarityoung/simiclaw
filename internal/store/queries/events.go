package queries

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (r *Repository) GetEventRecord(ctx context.Context, eventID string) (querymodel.EventRecord, bool, error) {
	rows, err := r.db.Reader().QueryContext(ctx, eventSelectSQL+` WHERE event_id = ?`, eventID)
	if err != nil {
		return querymodel.EventRecord{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return querymodel.EventRecord{}, false, rows.Err()
	}
	rec, err := scanEvent(rows)
	if err != nil {
		return querymodel.EventRecord{}, false, err
	}
	return rec, true, rows.Err()
}

func (r *Repository) LookupEvent(ctx context.Context, key string) (querymodel.LookupEvent, bool, error) {
	var row querymodel.LookupEvent
	var createdAt string
	err := r.db.Reader().QueryRowContext(
		ctx,
		`SELECT event_id, payload_hash, session_key, session_id, created_at FROM idempotency_keys WHERE key = ?`,
		key,
	).Scan(&row.EventID, &row.PayloadHash, &row.SessionKey, &row.SessionID, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return querymodel.LookupEvent{}, false, nil
	}
	if err != nil {
		return querymodel.LookupEvent{}, false, err
	}
	row.ReceivedAt = mustParseTime(createdAt)
	return row, true, nil
}

func (r *Repository) ListEventRecords(ctx context.Context, filter querymodel.EventFilter) ([]querymodel.EventRecord, error) {
	query := eventSelectSQL + ` WHERE 1 = 1`
	args := make([]any, 0, 6)
	if strings.TrimSpace(filter.SessionKey) != "" {
		query += ` AND session_key = ?`
		args = append(args, filter.SessionKey)
	}
	if filter.Status != "" {
		query += ` AND status = ?`
		args = append(args, string(filter.Status))
	}
	if filter.Cursor != nil && !filter.Cursor.CreatedAt.IsZero() && strings.TrimSpace(filter.Cursor.EventID) != "" {
		cursor := timeText(filter.Cursor.CreatedAt)
		query += ` AND (created_at < ? OR (created_at = ? AND event_id < ?))`
		args = append(args, cursor, cursor, filter.Cursor.EventID)
	}
	query += ` ORDER BY created_at DESC, event_id DESC LIMIT ?`
	args = append(args, normalizeListLimit(filter.Limit))

	rows, err := r.db.Reader().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]querymodel.EventRecord, 0, filter.Limit)
	for rows.Next() {
		rec, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func scanEvent(rows *sql.Rows) (querymodel.EventRecord, error) {
	var (
		rec          querymodel.EventRecord
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
		return querymodel.EventRecord{}, err
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
