package workers

import (
	"context"
	"fmt"
	"time"

	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const delayedPollTick = time.Second

type EventIngestor interface {
	Ingest(ctx context.Context, req IngestRequest) (IngestResult, error)
}

type IngestRequest struct {
	Source         string
	Conversation   model.Conversation
	IdempotencyKey string
	Timestamp      time.Time
	Payload        model.EventPayload
}

type IngestResult struct {
	EventID   string
	Duplicate bool
	Enqueued  bool
}

type ScheduledJobRepository interface {
	BeatHeartbeat(ctx context.Context, workerName string, now time.Time) error
	ClaimRuntimeScheduledJob(ctx context.Context, kind model.ScheduledJobKind, owner string, now time.Time) (runtimemodel.ClaimedJob, bool, error)
	FailScheduledJob(ctx context.Context, jobID, message string, nextRunAt, now time.Time) error
	CompleteRuntimeScheduledJob(ctx context.Context, job runtimemodel.ClaimedJob, now time.Time) error
	MarkEventQueued(ctx context.Context, eventID string, now time.Time) error
}

type ScheduledJobsWorker struct {
	repo   ScheduledJobRepository
	ingest EventIngestor
	queue  EventEnqueuer
	now    func() time.Time
	role   kernel.WorkerRole
	kinds  []model.ScheduledJobKind
	logger *logging.Logger
}

func NewDelayedJobsWorker(repo ScheduledJobRepository, ingestor EventIngestor, queue EventEnqueuer) *ScheduledJobsWorker {
	return &ScheduledJobsWorker{
		repo:   repo,
		ingest: ingestor,
		queue:  queue,
		now:    utcNow,
		role: kernel.WorkerRole{
			Name:          "scheduled_jobs_delayed",
			HeartbeatName: "delayed_jobs",
			PollCadence:   delayedPollTick,
			FailurePolicy: "retry failed jobs after cooldown and best-effort fallback enqueue",
		},
		kinds: []model.ScheduledJobKind{
			model.ScheduledJobKindDelayed,
			model.ScheduledJobKindRetry,
		},
		logger: logging.L("runtime.worker").With(logging.String("worker", "scheduled_jobs_delayed")),
	}
}

func NewCronWorker(repo ScheduledJobRepository, ingestor EventIngestor, queue EventEnqueuer) *ScheduledJobsWorker {
	return &ScheduledJobsWorker{
		repo:   repo,
		ingest: ingestor,
		queue:  queue,
		now:    utcNow,
		role: kernel.WorkerRole{
			Name:          "scheduled_jobs_cron",
			HeartbeatName: "cron",
			PollCadence:   delayedPollTick,
			FailurePolicy: "retry cron ingest failures after cooldown",
		},
		kinds: []model.ScheduledJobKind{
			model.ScheduledJobKindCron,
		},
		logger: logging.L("runtime.worker").With(logging.String("worker", "scheduled_jobs_cron")),
	}
}

func (w *ScheduledJobsWorker) Role() kernel.WorkerRole {
	return w.role
}

func (w *ScheduledJobsWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.role.PollCadence)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			now := w.now()
			if w.repo != nil {
				if err := w.repo.BeatHeartbeat(ctx, w.role.HeartbeatName, now); err != nil {
					w.logger.Warn("scheduled job heartbeat failed", logging.Error(err))
				}
			}
			for _, kind := range w.kinds {
				runScheduledKind(ctx, w.repo, w.ingest, w.queue, kind, now, w.logger.With(logging.String("job_kind", string(kind))))
			}
		}
	}
}

func RunScheduledKind(ctx context.Context, repo ScheduledJobRepository, ingestor EventIngestor, queue EventEnqueuer, kind model.ScheduledJobKind, now time.Time) {
	runScheduledKind(ctx, repo, ingestor, queue, kind, now, logging.L("runtime.worker").With(
		logging.String("worker", scheduledJobWorkerName(kind)),
		logging.String("job_kind", string(kind)),
	))
}

func runScheduledKind(ctx context.Context, repo ScheduledJobRepository, ingestor EventIngestor, queue EventEnqueuer, kind model.ScheduledJobKind, now time.Time, logger *logging.Logger) {
	if repo == nil {
		return
	}
	job, ok, err := repo.ClaimRuntimeScheduledJob(ctx, kind, string(kind)+"-worker", now)
	if err != nil {
		logger.Warn("job claim failed", logging.Error(err))
		return
	}
	if !ok {
		logger.Debug("job idle")
		return
	}
	jobLogger := logger.With(
		logging.String("job_id", job.JobID),
		logging.String("payload_type", job.Payload.Payload.Type),
	)
	jobLogger.Info("job claimed")
	req := IngestRequest{
		Source:         job.Payload.Source,
		Conversation:   job.Payload.Conversation,
		IdempotencyKey: fmt.Sprintf("%s:%d", job.JobID, now.Unix()),
		Timestamp:      now,
		Payload:        job.Payload.Payload,
	}
	if ingestor == nil {
		jobLogger.Error("job ingest unavailable", logging.Any("next_run_at", now.Add(30*time.Second)))
		_ = repo.FailScheduledJob(ctx, job.JobID, "ingest service unavailable", now.Add(30*time.Second), now)
		return
	}
	accepted, err := ingestor.Ingest(ctx, req)
	if err != nil {
		jobLogger.Warn("job ingest failed", logging.Error(err), logging.Any("next_run_at", now.Add(30*time.Second)))
		_ = repo.FailScheduledJob(ctx, job.JobID, err.Error(), now.Add(30*time.Second), now)
		return
	}
	jobLogger = jobLogger.With(logging.String("event_id", accepted.EventID))
	if err := repo.CompleteRuntimeScheduledJob(ctx, job, now); err != nil {
		jobLogger.Warn("job completion record failed", logging.Error(err))
	}
	if !accepted.Duplicate && !accepted.Enqueued && queue != nil {
		if err := repo.MarkEventQueued(ctx, accepted.EventID, now); err == nil {
			if queue.TryEnqueue(accepted.EventID) {
				jobLogger.Info("job fallback enqueued", logging.Bool("duplicate", false), logging.Bool("enqueued", true))
			} else {
				jobLogger.Warn("job accepted but not enqueued", logging.Bool("duplicate", false), logging.Bool("enqueued", false))
			}
		} else {
			jobLogger.Warn("mark event queued failed", logging.Error(err))
		}
	} else if accepted.Duplicate {
		jobLogger.Info("job duplicate", logging.Bool("duplicate", true), logging.Bool("enqueued", accepted.Enqueued))
	} else if accepted.Enqueued {
		jobLogger.Info("job enqueued", logging.Bool("duplicate", false), logging.Bool("enqueued", true))
	} else {
		jobLogger.Warn("job accepted but not enqueued", logging.Bool("duplicate", false), logging.Bool("enqueued", false))
	}
	_ = repo.BeatHeartbeat(ctx, string(kind), now)
}

func scheduledJobWorkerName(kind model.ScheduledJobKind) string {
	switch kind {
	case model.ScheduledJobKindCron:
		return "scheduled_jobs_cron"
	default:
		return "scheduled_jobs_delayed"
	}
}
