package tx

import (
	"context"
	"time"
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
	return r.db.RecoverExpiredProcessing(ctx, cutoff, now)
}
