package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type ScheduledJobPayload struct {
	Source          string             `json:"source"`
	TenantID        string             `json:"tenant_id"`
	Conversation    model.Conversation `json:"conversation"`
	Payload         model.EventPayload `json:"payload"`
	IntervalSeconds int64              `json:"interval_seconds,omitempty"`
}

type ClaimedJob struct {
	JobID        string
	Name         string
	Kind         model.ScheduledJobKind
	Status       model.ScheduledJobStatus
	Payload      ScheduledJobPayload
	AttemptCount int
	NextRunAt    time.Time
}

func (db *DB) UpsertCronJobs(ctx context.Context, tenantID string, jobs []config.CronJobConfig, now time.Time) error {
	return db.WithWriterTx(ctx, func(tx *sql.Tx) error {
		for _, job := range jobs {
			payload, err := json.Marshal(ScheduledJobPayload{
				Source:   "cron",
				TenantID: tenantID,
				Conversation: model.Conversation{
					ConversationID: job.ConversationID,
					ChannelType:    job.ChannelType,
					ParticipantID:  job.ParticipantID,
				},
				Payload: model.EventPayload{
					Type: job.PayloadType,
					Text: job.PayloadText,
				},
				IntervalSeconds: int64(job.Interval.Duration / time.Second),
			})
			if err != nil {
				return err
			}
			jobID := "cron:" + job.Name
			_, err = tx.ExecContext(
				ctx,
				`INSERT INTO scheduled_jobs (
					job_id, name, kind, status, payload_json, next_run_at, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(job_id) DO UPDATE SET
					name = excluded.name,
					payload_json = excluded.payload_json,
					updated_at = excluded.updated_at`,
				jobID,
				job.Name,
				string(model.ScheduledJobKindCron),
				string(model.ScheduledJobStatusActive),
				string(payload),
				timeText(now.Add(job.Interval.Duration)),
				timeText(now),
				timeText(now),
			)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (db *DB) ClaimScheduledJob(ctx context.Context, kind model.ScheduledJobKind, owner string, now time.Time) (ClaimedJob, bool, error) {
	row := db.writer.QueryRowContext(
		ctx,
		`UPDATE scheduled_jobs
		 SET locked_at = ?, lock_owner = ?, updated_at = ?, attempt_count = attempt_count + 1
		 WHERE job_id = (
		     SELECT job_id FROM scheduled_jobs
		     WHERE kind = ? AND status = ? AND next_run_at <= ?
		     ORDER BY next_run_at ASC, job_id ASC
		     LIMIT 1
		 )
		 RETURNING job_id, name, kind, status, payload_json, attempt_count, next_run_at`,
		timeText(now),
		owner,
		timeText(now),
		string(kind),
		string(model.ScheduledJobStatusActive),
		timeText(now),
	)
	var (
		job         ClaimedJob
		payloadJSON string
		nextRunAt   string
	)
	err := row.Scan(&job.JobID, &job.Name, &job.Kind, &job.Status, &payloadJSON, &job.AttemptCount, &nextRunAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ClaimedJob{}, false, nil
	}
	if err != nil {
		return ClaimedJob{}, false, err
	}
	if err := json.Unmarshal([]byte(payloadJSON), &job.Payload); err != nil {
		return ClaimedJob{}, false, err
	}
	job.NextRunAt = mustParseTime(nextRunAt)
	return job, true, nil
}

func (db *DB) CompleteScheduledJob(ctx context.Context, job ClaimedJob, now time.Time) error {
	return db.WithWriterTx(ctx, func(tx *sql.Tx) error {
		nextRunAt := ""
		status := string(job.Status)
		switch job.Kind {
		case model.ScheduledJobKindCron:
			interval := time.Duration(job.Payload.IntervalSeconds) * time.Second
			if interval <= 0 {
				interval = 10 * time.Second
			}
			nextRunAt = timeText(now.Add(interval))
		default:
			status = string(model.ScheduledJobStatusPaused)
		}
		_, err := tx.ExecContext(
			ctx,
			`UPDATE scheduled_jobs
			 SET status = ?, next_run_at = ?, locked_at = '', lock_owner = '', last_error = '', updated_at = ?
			 WHERE job_id = ?`,
			status,
			nextRunAt,
			timeText(now),
			job.JobID,
		)
		return err
	})
}

func (db *DB) FailScheduledJob(ctx context.Context, jobID, message string, nextRunAt, now time.Time) error {
	_, err := db.writer.ExecContext(
		ctx,
		`UPDATE scheduled_jobs
		 SET locked_at = '', lock_owner = '', last_error = ?, next_run_at = ?, updated_at = ?
		 WHERE job_id = ?`,
		message,
		timeText(nextRunAt),
		timeText(now),
		jobID,
	)
	return err
}

func (db *DB) BeatHeartbeat(ctx context.Context, workerName string, now time.Time) error {
	_, err := db.writer.ExecContext(
		ctx,
		`INSERT INTO heartbeats (worker_name, beat_at, status, details)
		 VALUES (?, ?, 'alive', '')
		 ON CONFLICT(worker_name) DO UPDATE SET beat_at = excluded.beat_at, status = excluded.status, details = excluded.details`,
		workerName,
		timeText(now),
	)
	return err
}

func (db *DB) HeartbeatAt(ctx context.Context, workerName string) (time.Time, bool, error) {
	var raw string
	err := db.reader.QueryRowContext(ctx, `SELECT beat_at FROM heartbeats WHERE worker_name = ?`, workerName).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return mustParseTime(raw), true, nil
}
