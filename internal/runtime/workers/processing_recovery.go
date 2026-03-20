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

// EventEnqueuer 定义了一个接口，用于尝试将事件重新加入处理队列。实现者可以根据实际情况决定是否成功入队。
type EventEnqueuer interface {
	TryEnqueue(eventID string) bool
}

// ProcessingRecoveryRepository 定义了处理恢复所需的存储接口，包括心跳更新和过期处理恢复。
type ProcessingRecoveryRepository interface {
	BeatHeartbeat(ctx context.Context, workerName string, now time.Time) error
	RecoverExpiredProcessing(ctx context.Context, cutoff, now time.Time) ([]string, error)
}

// ProcessingRecoveryWorker 定义了一个工作者，用于定期检查和恢复过期的处理事件。它依赖于一个存储库来管理心跳和恢复逻辑，以及一个事件入队器来重新加入处理队列。
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
