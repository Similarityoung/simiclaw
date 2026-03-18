package workers

import (
	"context"
	"fmt"
	"time"

	"github.com/similarityyoung/simiclaw/internal/ingest"
	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const delayedPollTick = time.Second

type EventIngestor interface {
	Ingest(ctx context.Context, cmd ingest.Command) (ingest.Result, *ingest.Error)
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
				_ = w.repo.BeatHeartbeat(ctx, w.role.HeartbeatName, now)
			}
			for _, kind := range w.kinds {
				RunScheduledKind(ctx, w.repo, w.ingest, w.queue, kind, now)
			}
		}
	}
}

func RunScheduledKind(ctx context.Context, repo ScheduledJobRepository, ingestor EventIngestor, queue EventEnqueuer, kind model.ScheduledJobKind, now time.Time) {
	if repo == nil {
		return
	}
	job, ok, err := repo.ClaimRuntimeScheduledJob(ctx, kind, string(kind)+"-worker", now)
	if err != nil || !ok {
		return
	}
	req := api.IngestRequest{
		Source:         job.Payload.Source,
		Conversation:   job.Payload.Conversation,
		IdempotencyKey: fmt.Sprintf("%s:%d", job.JobID, now.Unix()),
		Timestamp:      now.Format(time.RFC3339Nano),
		Payload:        job.Payload.Payload,
	}
	if ingestor == nil {
		_ = repo.FailScheduledJob(ctx, job.JobID, "ingest service unavailable", now.Add(30*time.Second), now)
		return
	}
	result, ingestErr := ingestor.Ingest(ctx, ingest.Command{Request: req, ReceivedAt: now})
	if ingestErr != nil {
		_ = repo.FailScheduledJob(ctx, job.JobID, ingestErr.Error(), now.Add(30*time.Second), now)
		return
	}
	_ = repo.CompleteRuntimeScheduledJob(ctx, job, now)
	if !result.Duplicate && !result.Enqueued && queue != nil {
		if err := repo.MarkEventQueued(ctx, result.EventID, now); err == nil {
			queue.TryEnqueue(result.EventID)
		}
	}
	_ = repo.BeatHeartbeat(ctx, string(kind), now)
}
