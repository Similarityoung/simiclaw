package workers

import (
	"context"
	"time"

	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const (
	outboxSendingLease = 30 * time.Second
	outboxPollTick     = 500 * time.Millisecond
)

type Sender interface {
	Send(ctx context.Context, msg model.OutboxMessage) error
}

type DeliveryPollRepository interface {
	BeatHeartbeat(ctx context.Context, workerName string, now time.Time) error
	RecoverExpiredSending(ctx context.Context, cutoff, now time.Time) error
	ClaimRuntimeOutbox(ctx context.Context, owner string, now time.Time) (runtimemodel.ClaimedOutbox, bool, error)
	FailOutboxSend(ctx context.Context, outboxID, eventID, message string, dead bool, nextAttemptAt, now time.Time) error
	CompleteOutboxSend(ctx context.Context, outboxID, eventID string, now time.Time) error
}

type DeliveryPollWorker struct {
	repo   DeliveryPollRepository
	sender Sender
	now    func() time.Time
	role   kernel.WorkerRole
}

func NewDeliveryPollWorker(repo DeliveryPollRepository, sender Sender) *DeliveryPollWorker {
	return &DeliveryPollWorker{
		repo:   repo,
		sender: sender,
		now:    utcNow,
		role: kernel.WorkerRole{
			Name:          "delivery_poll",
			HeartbeatName: "outbox_retry",
			PollCadence:   outboxPollTick,
			FailurePolicy: "retry with bounded backoff and dead-letter after max attempts",
		},
	}
}

func (w *DeliveryPollWorker) Role() kernel.WorkerRole {
	return w.role
}

func (w *DeliveryPollWorker) Run(ctx context.Context) error {
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
				_ = w.repo.RecoverExpiredSending(ctx, now.Add(-outboxSendingLease), now)
			}
			if w.repo == nil {
				continue
			}
			msg, ok, err := w.repo.ClaimRuntimeOutbox(ctx, "outbox-worker", now)
			if err != nil || !ok {
				continue
			}
			if err := w.sender.Send(ctx, model.OutboxMessage{
				OutboxID:   msg.OutboxID,
				EventID:    msg.EventID,
				SessionKey: msg.SessionKey,
				Channel:    msg.Channel,
				TargetID:   msg.TargetID,
				Body:       msg.Body,
				CreatedAt:  msg.CreatedAt,
				Attempts:   msg.AttemptCount,
			}); err != nil {
				backoff := time.Duration(msg.AttemptCount+1) * 5 * time.Second
				if backoff > 5*time.Minute {
					backoff = 5 * time.Minute
				}
				dead := msg.AttemptCount >= 5
				_ = w.repo.FailOutboxSend(ctx, msg.OutboxID, msg.EventID, err.Error(), dead, now.Add(backoff), now)
				continue
			}
			_ = w.repo.CompleteOutboxSend(ctx, msg.OutboxID, msg.EventID, now)
		}
	}
}
