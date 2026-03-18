package tx

import (
	"context"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
)

func (r *RuntimeRepository) GetEventRecord(ctx context.Context, eventID string) (runtimemodel.EventRecord, bool, error) {
	rec, ok, err := r.db.GetEvent(ctx, eventID)
	if err != nil || !ok {
		return runtimemodel.EventRecord{}, ok, err
	}
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
	}, true, nil
}
