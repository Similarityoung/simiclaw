package workers

import (
	"context"
	"time"

	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
)

const heartbeatInterval = 10 * time.Second

type HeartbeatRepository interface {
	BeatHeartbeat(ctx context.Context, workerName string, now time.Time) error
}

type HeartbeatWorker struct {
	repo HeartbeatRepository
	now  func() time.Time
	role kernel.WorkerRole
}

func NewHeartbeatWorker(repo HeartbeatRepository) *HeartbeatWorker {
	return &HeartbeatWorker{
		repo: repo,
		now:  utcNow,
		role: kernel.WorkerRole{
			Name:          "heartbeat",
			HeartbeatName: "heartbeat",
			PollCadence:   heartbeatInterval,
			FailurePolicy: "best-effort heartbeat updates",
		},
	}
}

func (w *HeartbeatWorker) Role() kernel.WorkerRole {
	return w.role
}

func (w *HeartbeatWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.role.PollCadence)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if w.repo != nil {
				_ = w.repo.BeatHeartbeat(ctx, w.role.HeartbeatName, w.now())
			}
		}
	}
}

func utcNow() time.Time {
	return time.Now().UTC()
}
