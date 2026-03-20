package delivery

import (
	"context"
	"time"

	"github.com/similarityyoung/simiclaw/internal/outbound/retry"
	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const (
	sendingLease = 30 * time.Second
	pollTick     = 500 * time.Millisecond
	sendTimeout  = 30 * time.Second
)

type Sender interface {
	Send(ctx context.Context, msg model.OutboxMessage) error
}

type Repository interface {
	BeatHeartbeat(ctx context.Context, workerName string, now time.Time) error
	RecoverExpiredSending(ctx context.Context, cutoff, now time.Time) error
	ClaimRuntimeOutbox(ctx context.Context, owner string, now time.Time) (runtimemodel.ClaimedOutbox, bool, error)
	FailOutboxSend(ctx context.Context, outboxID, eventID, message string, dead bool, nextAttemptAt, now time.Time) error
	CompleteOutboxSend(ctx context.Context, outboxID, eventID string, now time.Time) error
}

type Worker struct {
	repo              Repository
	sender            Sender
	retryPolicy       retry.Policy
	now               func() time.Time
	sendTimeout       time.Duration
	role              kernel.WorkerRole
	logger            *logging.Logger
	missingSenderWarn bool
}

func NewWorker(repo Repository, sender Sender) *Worker {
	return &Worker{
		repo:        repo,
		sender:      sender,
		retryPolicy: retry.DefaultPolicy(),
		now: func() time.Time {
			return time.Now().UTC()
		},
		sendTimeout: sendTimeout,
		role: kernel.WorkerRole{
			Name:          "delivery_poll",
			HeartbeatName: "outbox_retry",
			PollCadence:   pollTick,
			FailurePolicy: "retry with bounded exponential backoff and dead-letter after max attempts",
		},
		logger: logging.L("outbound.delivery"),
	}
}

func (w *Worker) Role() kernel.WorkerRole {
	return w.role
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.role.PollCadence)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	now := w.now()
	if w.repo != nil {
		if err := w.repo.BeatHeartbeat(ctx, w.role.HeartbeatName, now); err != nil {
			w.logger.Warn("heartbeat failed", logging.Error(err))
		}
		if err := w.repo.RecoverExpiredSending(ctx, now.Add(-sendingLease), now); err != nil {
			w.logger.Warn("recover expired sending failed", logging.Error(err))
		}
	}
	if w.repo == nil {
		return
	}
	if w.sender == nil {
		if !w.missingSenderWarn {
			w.logger.Error("sender not configured")
			w.missingSenderWarn = true
		}
		return
	}

	msg, ok, err := w.repo.ClaimRuntimeOutbox(ctx, "outbox-worker", now)
	if err != nil {
		w.logger.Warn("claim outbox failed", logging.Error(err))
		return
	}
	if !ok {
		return
	}

	logger := w.logger.With(
		logging.String("outbox_id", msg.OutboxID),
		logging.String("event_id", msg.EventID),
		logging.String("session_key", msg.SessionKey),
		logging.String("channel", msg.Channel),
		logging.String("target_id", msg.TargetID),
		logging.Int("attempt_count", msg.AttemptCount),
	)
	logger.Info("send started")
	if err := w.send(ctx, msg); err != nil {
		decision := w.retryPolicy.Next(msg.AttemptCount, now)
		if recordErr := w.repo.FailOutboxSend(ctx, msg.OutboxID, msg.EventID, err.Error(), decision.Dead, decision.NextAttemptAt, now); recordErr != nil {
			logger.Error("failed to record outbound send failure", logging.Error(recordErr), logging.NamedError("send_error", err))
			return
		}
		if decision.Dead {
			logger.Error("dead-lettered",
				logging.Error(err),
				logging.Bool("dead", true),
			)
		} else {
			logger.Warn("retry scheduled",
				logging.Error(err),
				logging.Bool("dead", false),
				logging.Any("next_attempt_at", decision.NextAttemptAt),
			)
		}
		return
	}

	if err := w.repo.CompleteOutboxSend(ctx, msg.OutboxID, msg.EventID, now); err != nil {
		logger.Error("failed to mark outbound send completed", logging.Error(err))
		return
	}
	logger.Info("sent")
}

func (w *Worker) send(ctx context.Context, msg runtimemodel.ClaimedOutbox) error {
	sendCtx := ctx
	cancel := func() {}
	if w.sendTimeout > 0 {
		sendCtx, cancel = context.WithTimeout(ctx, w.sendTimeout)
	}
	defer cancel()

	return w.sender.Send(sendCtx, model.OutboxMessage{
		OutboxID:   msg.OutboxID,
		EventID:    msg.EventID,
		SessionKey: msg.SessionKey,
		Channel:    msg.Channel,
		TargetID:   msg.TargetID,
		Body:       msg.Body,
		CreatedAt:  msg.CreatedAt,
		Attempts:   msg.AttemptCount,
	})
}
