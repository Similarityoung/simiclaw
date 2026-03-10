package store

import (
	"context"
	"time"

	"github.com/similarityyoung/simiclaw/internal/readmodel"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (db *DB) ClaimLoopEvent(ctx context.Context, eventID, runID string, now time.Time) (runtimemodel.ClaimedEvent, bool, error) {
	claimed, ok, err := db.ClaimEvent(ctx, eventID, runID, now)
	if err != nil || !ok {
		return runtimemodel.ClaimedEvent{}, ok, err
	}
	return runtimemodel.ClaimedEvent{
		Event:   claimed.Event,
		RunID:   claimed.RunID,
		Status:  claimed.Status,
		RunMode: claimed.RunMode,
	}, true, nil
}

func (db *DB) FinalizeLoopRun(ctx context.Context, finalize runtimemodel.RunFinalize) error {
	return db.FinalizeRun(ctx, toStoreRunFinalize(finalize))
}

func (db *DB) GetLoopEventRecord(ctx context.Context, eventID string) (runtimemodel.EventRecord, bool, error) {
	rec, ok, err := db.GetEvent(ctx, eventID)
	if err != nil || !ok {
		return runtimemodel.EventRecord{}, ok, err
	}
	return toRuntimeEventRecord(rec), true, nil
}

func toStoreRunFinalize(in runtimemodel.RunFinalize) RunFinalize {
	out := RunFinalize{
		RunID:             in.RunID,
		EventID:           in.EventID,
		SessionKey:        in.SessionKey,
		SessionID:         in.SessionID,
		RunMode:           in.RunMode,
		RunStatus:         in.RunStatus,
		EventStatus:       in.EventStatus,
		Provider:          in.Provider,
		Model:             in.Model,
		PromptTokens:      in.PromptTokens,
		CompletionTokens:  in.CompletionTokens,
		TotalTokens:       in.TotalTokens,
		LatencyMS:         in.LatencyMS,
		FinishReason:      in.FinishReason,
		RawFinishReason:   in.RawFinishReason,
		ProviderRequestID: in.ProviderRequestID,
		OutputText:        in.OutputText,
		ToolCalls:         in.ToolCalls,
		Diagnostics:       in.Diagnostics,
		Error:             in.Error,
		AssistantReply:    in.AssistantReply,
		OutboxChannel:     in.OutboxChannel,
		OutboxTargetID:    in.OutboxTargetID,
		OutboxBody:        in.OutboxBody,
		Now:               in.Now,
	}
	out.Messages = make([]StoredMessage, 0, len(in.Messages))
	for _, msg := range in.Messages {
		out.Messages = append(out.Messages, StoredMessage{
			MessageID:  msg.MessageID,
			SessionKey: msg.SessionKey,
			SessionID:  msg.SessionID,
			RunID:      msg.RunID,
			Role:       msg.Role,
			Content:    msg.Content,
			Visible:    msg.Visible,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
			ToolName:   msg.ToolName,
			ToolArgs:   msg.ToolArgs,
			ToolResult: msg.ToolResult,
			Meta:       msg.Meta,
			CreatedAt:  msg.CreatedAt,
		})
	}
	return out
}

func toRuntimeEventRecord(rec readmodel.EventRecord) runtimemodel.EventRecord {
	return runtimemodel.EventRecord{
		EventID:           rec.EventID,
		Status:            rec.Status,
		OutboxStatus:      rec.OutboxStatus,
		SessionKey:        rec.SessionKey,
		SessionID:         rec.SessionID,
		RunID:             rec.RunID,
		RunMode:           rec.RunMode,
		AssistantReply:    rec.AssistantReply,
		OutboxID:          rec.OutboxID,
		ProcessingLease:   rec.ProcessingLease,
		ReceivedAt:        rec.ReceivedAt,
		CreatedAt:         rec.CreatedAt,
		UpdatedAt:         rec.UpdatedAt,
		PayloadHash:       rec.PayloadHash,
		Provider:          rec.Provider,
		Model:             rec.Model,
		ProviderRequestID: rec.ProviderRequestID,
		Error:             rec.Error,
	}
}

func (db *DB) ClaimRuntimeOutbox(ctx context.Context, owner string, now time.Time) (runtimemodel.ClaimedOutbox, bool, error) {
	msg, ok, err := db.ClaimOutbox(ctx, owner, now)
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

func (db *DB) ClaimRuntimeScheduledJob(ctx context.Context, kind model.ScheduledJobKind, owner string, now time.Time) (runtimemodel.ClaimedJob, bool, error) {
	job, ok, err := db.ClaimScheduledJob(ctx, kind, owner, now)
	if err != nil || !ok {
		return runtimemodel.ClaimedJob{}, ok, err
	}
	return toRuntimeClaimedJob(job), true, nil
}

func (db *DB) CompleteRuntimeScheduledJob(ctx context.Context, job runtimemodel.ClaimedJob, now time.Time) error {
	return db.CompleteScheduledJob(ctx, toStoreClaimedJob(job), now)
}

func toRuntimeClaimedJob(job ClaimedJob) runtimemodel.ClaimedJob {
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

func toRuntimeScheduledJobPayload(payload ScheduledJobPayload) runtimemodel.ScheduledJobPayload {
	return runtimemodel.ScheduledJobPayload{
		Source:          payload.Source,
		TenantID:        payload.TenantID,
		Conversation:    payload.Conversation,
		Payload:         payload.Payload,
		IntervalSeconds: payload.IntervalSeconds,
	}
}

func toStoreClaimedJob(job runtimemodel.ClaimedJob) ClaimedJob {
	return ClaimedJob{
		JobID:        job.JobID,
		Name:         job.Name,
		Kind:         job.Kind,
		Status:       job.Status,
		Payload:      toStoreScheduledJobPayload(job.Payload),
		AttemptCount: job.AttemptCount,
		NextRunAt:    job.NextRunAt,
	}
}

func toStoreScheduledJobPayload(payload runtimemodel.ScheduledJobPayload) ScheduledJobPayload {
	return ScheduledJobPayload{
		Source:          payload.Source,
		TenantID:        payload.TenantID,
		Conversation:    payload.Conversation,
		Payload:         payload.Payload,
		IntervalSeconds: payload.IntervalSeconds,
	}
}
