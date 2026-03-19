package tx

import (
	"context"
	"database/sql"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (r *RuntimeRepository) CheckReadWrite(ctx context.Context) error {
	return r.db.CheckReadWrite(ctx)
}

func (r *RuntimeRepository) BeatHeartbeat(ctx context.Context, workerName string, now time.Time) error {
	return r.db.BeatHeartbeat(ctx, workerName, now)
}

func (r *RuntimeRepository) HeartbeatAt(ctx context.Context, workerName string) (time.Time, bool, error) {
	return r.db.HeartbeatAt(ctx, workerName)
}

func (r *RuntimeRepository) RecoverExpiredProcessing(ctx context.Context, cutoff, now time.Time) ([]string, error) {
	rows, err := r.db.Reader().QueryContext(
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
		if err := r.db.WithWriterTx(ctx, func(tx *sql.Tx) error {
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
