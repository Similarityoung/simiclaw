package tx

import (
	"context"
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

func (r *RuntimeRepository) RecoverExpiredSending(ctx context.Context, cutoff, now time.Time) error {
	return r.db.RecoverExpiredSending(ctx, cutoff, now)
}

func (r *RuntimeRepository) ClaimRuntimeOutbox(ctx context.Context, owner string, now time.Time) (runtimemodel.ClaimedOutbox, bool, error) {
	msg, ok, err := r.db.ClaimOutbox(ctx, owner, now)
	if err != nil || !ok {
		return runtimemodel.ClaimedOutbox{}, ok, err
	}
	return runtimemodel.ClaimedOutbox{
		OutboxID:     msg.OutboxID,
		EventID:      msg.EventID,
		SessionKey:   msg.SessionKey,
		Channel:      msg.Channel,
		TargetID:     msg.TargetID,
		Body:         msg.Body,
		AttemptCount: msg.AttemptCount,
		CreatedAt:    msg.CreatedAt,
	}, true, nil
}

func (r *RuntimeRepository) FailOutboxSend(ctx context.Context, outboxID, eventID, message string, dead bool, nextAttemptAt, now time.Time) error {
	return r.db.FailOutboxSend(ctx, outboxID, eventID, message, dead, nextAttemptAt, now)
}

func (r *RuntimeRepository) CompleteOutboxSend(ctx context.Context, outboxID, eventID string, now time.Time) error {
	return r.db.CompleteOutboxSend(ctx, outboxID, eventID, now)
}
