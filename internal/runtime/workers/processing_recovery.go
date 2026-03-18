package workers

import (
	"context"
	"time"

	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
)

const (
	processingLease     = 2 * time.Minute
	processingSweepTick = 10 * time.Second
)

type EventEnqueuer interface {
	TryEnqueue(eventID string) bool
}

type ProcessingRecoveryRepository interface {
	BeatHeartbeat(ctx context.Context, workerName string, now time.Time) error
	RecoverExpiredProcessing(ctx context.Context, cutoff, now time.Time) ([]string, error)
}

type ProcessingRecoveryWorker struct {
	repo  ProcessingRecoveryRepository
	queue EventEnqueuer
	now   func() time.Time
	role  kernel.WorkerRole
}

func NewProcessingRecoveryWorker(repo ProcessingRecoveryRepository, queue EventEnqueuer) *ProcessingRecoveryWorker {
	return &ProcessingRecoveryWorker{
		repo:  repo,
		queue: queue,
		now:   utcNow,
		role: kernel.WorkerRole{
			Name:          "processing_recovery",
			HeartbeatName: "processing_sweeper",
			PollCadence:   processingSweepTick,
			FailurePolicy: "continue on recover errors and requeue best-effort",
		},
	}
}

func (w *ProcessingRecoveryWorker) Role() kernel.WorkerRole {
	return w.role
}

func (w *ProcessingRecoveryWorker) Run(ctx context.Context) error {
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
			if w.repo == nil {
				continue
			}
			ids, err := w.repo.RecoverExpiredProcessing(ctx, now.Add(-processingLease), now)
			if err != nil {
				continue
			}
			for _, eventID := range ids {
				if w.queue != nil {
					w.queue.TryEnqueue(eventID)
				}
			}
		}
	}
}
