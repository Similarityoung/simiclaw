package tx

import (
	"context"
	"time"

	"github.com/similarityyoung/simiclaw/internal/config"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (r *RuntimeRepository) UpsertCronJobs(ctx context.Context, tenantID string, jobs []config.CronJobConfig, now time.Time) error {
	return r.db.UpsertCronJobs(ctx, tenantID, jobs, now)
}

func (r *RuntimeRepository) ClaimRuntimeScheduledJob(ctx context.Context, kind model.ScheduledJobKind, owner string, now time.Time) (runtimemodel.ClaimedJob, bool, error) {
	job, ok, err := r.db.ClaimScheduledJob(ctx, kind, owner, now)
	if err != nil || !ok {
		return runtimemodel.ClaimedJob{}, ok, err
	}
	return toRuntimeClaimedJob(job), true, nil
}

func (r *RuntimeRepository) CompleteRuntimeScheduledJob(ctx context.Context, job runtimemodel.ClaimedJob, now time.Time) error {
	return r.db.CompleteScheduledJob(ctx, toStoreClaimedJob(job), now)
}

func (r *RuntimeRepository) FailScheduledJob(ctx context.Context, jobID, message string, nextRunAt, now time.Time) error {
	return r.db.FailScheduledJob(ctx, jobID, message, nextRunAt, now)
}

func toRuntimeClaimedJob(job store.ClaimedJob) runtimemodel.ClaimedJob {
	return runtimemodel.ClaimedJob{
		JobID:        job.JobID,
		Name:         job.Name,
		Kind:         job.Kind,
		Status:       job.Status,
		Payload:      toRuntimeScheduledJobPayload(job.Payload),
		AttemptCount: job.AttemptCount,
		NextRunAt:    job.NextRunAt,
	}
}

func toRuntimeScheduledJobPayload(payload store.ScheduledJobPayload) runtimemodel.ScheduledJobPayload {
	return runtimemodel.ScheduledJobPayload{
		Source:          payload.Source,
		TenantID:        payload.TenantID,
		Conversation:    payload.Conversation,
		Payload:         payload.Payload,
		IntervalSeconds: payload.IntervalSeconds,
	}
}

func toStoreClaimedJob(job runtimemodel.ClaimedJob) store.ClaimedJob {
	return store.ClaimedJob{
		JobID:        job.JobID,
		Name:         job.Name,
		Kind:         job.Kind,
		Status:       job.Status,
		Payload:      toStoreScheduledJobPayload(job.Payload),
		AttemptCount: job.AttemptCount,
		NextRunAt:    job.NextRunAt,
	}
}

func toStoreScheduledJobPayload(payload runtimemodel.ScheduledJobPayload) store.ScheduledJobPayload {
	return store.ScheduledJobPayload{
		Source:          payload.Source,
		TenantID:        payload.TenantID,
		Conversation:    payload.Conversation,
		Payload:         payload.Payload,
		IntervalSeconds: payload.IntervalSeconds,
	}
}
