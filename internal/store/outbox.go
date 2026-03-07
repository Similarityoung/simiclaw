package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (db *DB) RecoverExpiredSending(ctx context.Context, cutoff, now time.Time) error {
	return db.WithWriterTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(
			ctx,
			`SELECT outbox_id, event_id FROM outbox
			 WHERE status = ? AND locked_at != '' AND locked_at <= ?`,
			string(model.OutboxStatusSending),
			timeText(cutoff),
		)
		if err != nil {
			return err
		}
		defer rows.Close()
		type row struct {
			outboxID string
			eventID  string
		}
		var expired []row
		for rows.Next() {
			var item row
			if err := rows.Scan(&item.outboxID, &item.eventID); err != nil {
				return err
			}
			expired = append(expired, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, item := range expired {
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE outbox
				 SET status = ?, locked_at = '', lock_owner = '', next_attempt_at = ?, updated_at = ?
				 WHERE outbox_id = ?`,
				string(model.OutboxStatusRetryWait),
				timeText(now),
				timeText(now),
				item.outboxID,
			); err != nil {
				return err
			}
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE events SET outbox_status = ?, updated_at = ? WHERE event_id = ?`,
				string(model.OutboxStatusRetryWait),
				timeText(now),
				item.eventID,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

func (db *DB) ClaimOutbox(ctx context.Context, owner string, now time.Time) (ClaimedOutbox, bool, error) {
	row := db.writer.QueryRowContext(
		ctx,
		`UPDATE outbox
		 SET status = ?, locked_at = ?, lock_owner = ?, updated_at = ?
		 WHERE outbox_id = (
		     SELECT outbox_id FROM outbox
		     WHERE status IN (?, ?)
		       AND next_attempt_at <= ?
		     ORDER BY next_attempt_at ASC, outbox_id ASC
		     LIMIT 1
		 )
		 RETURNING outbox_id, event_id, session_key, body, attempt_count, created_at`,
		string(model.OutboxStatusSending),
		timeText(now),
		owner,
		timeText(now),
		string(model.OutboxStatusPending),
		string(model.OutboxStatusRetryWait),
		timeText(now),
	)
	var claimed ClaimedOutbox
	var createdAt string
	err := row.Scan(&claimed.OutboxID, &claimed.EventID, &claimed.SessionKey, &claimed.Body, &claimed.AttemptCount, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ClaimedOutbox{}, false, nil
	}
	if err != nil {
		return ClaimedOutbox{}, false, err
	}
	claimed.CreatedAt = mustParseTime(createdAt)
	return claimed, true, nil
}

func (db *DB) CompleteOutboxSend(ctx context.Context, outboxID, eventID string, now time.Time) error {
	return db.WithWriterTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE outbox
			 SET status = ?, locked_at = '', lock_owner = '', sent_at = ?, updated_at = ?
			 WHERE outbox_id = ?`,
			string(model.OutboxStatusSent),
			timeText(now),
			timeText(now),
			outboxID,
		); err != nil {
			return err
		}
		_, err := tx.ExecContext(
			ctx,
			`UPDATE events SET outbox_status = ?, updated_at = ? WHERE event_id = ?`,
			string(model.OutboxStatusSent),
			timeText(now),
			eventID,
		)
		return err
	})
}

func (db *DB) FailOutboxSend(ctx context.Context, outboxID, eventID, message string, dead bool, nextAttemptAt, now time.Time) error {
	status := model.OutboxStatusRetryWait
	if dead {
		status = model.OutboxStatusDead
	}
	return db.WithWriterTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE outbox
			 SET status = ?, locked_at = '', lock_owner = '', attempt_count = attempt_count + 1,
			     last_error = ?, next_attempt_at = ?, updated_at = ?
			 WHERE outbox_id = ?`,
			string(status),
			message,
			timeText(nextAttemptAt),
			timeText(now),
			outboxID,
		); err != nil {
			return err
		}
		_, err := tx.ExecContext(
			ctx,
			`UPDATE events SET outbox_status = ?, updated_at = ? WHERE event_id = ?`,
			string(status),
			timeText(now),
			eventID,
		)
		return err
	})
}
