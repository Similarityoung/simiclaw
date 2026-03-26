package bootstrap

import (
	"context"

	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type terminalEventQuery interface {
	GetEvent(ctx context.Context, eventID string) (querymodel.EventRecord, bool, error)
}

type runtimeTerminalReplaySource struct {
	query terminalEventQuery
}

func (s runtimeTerminalReplaySource) GetTerminalEvent(ctx context.Context, eventID string) (runtimemodel.RuntimeEvent, bool, error) {
	if s.query == nil {
		return runtimemodel.RuntimeEvent{}, false, nil
	}
	rec, ok, err := s.query.GetEvent(ctx, eventID)
	if err != nil || !ok {
		return runtimemodel.RuntimeEvent{}, false, err
	}
	event, ok := terminalRuntimeEventFromQueryRecord(rec)
	return event, ok, nil
}

func terminalRuntimeEventFromQueryRecord(rec querymodel.EventRecord) (runtimemodel.RuntimeEvent, bool) {
	eventRecord := queryEventRecordToRuntime(rec)
	switch rec.Status {
	case model.EventStatusFailed:
		return runtimemodel.RuntimeEvent{
			Kind:        runtimemodel.RuntimeEventFailed,
			EventID:     rec.EventID,
			RunID:       rec.RunID,
			SessionKey:  rec.SessionKey,
			SessionID:   rec.SessionID,
			OccurredAt:  rec.UpdatedAt,
			Error:       rec.Error,
			EventRecord: &eventRecord,
		}, true
	case model.EventStatusProcessed, model.EventStatusSuppressed:
		return runtimemodel.RuntimeEvent{
			Kind:        runtimemodel.RuntimeEventCompleted,
			EventID:     rec.EventID,
			RunID:       rec.RunID,
			SessionKey:  rec.SessionKey,
			SessionID:   rec.SessionID,
			OccurredAt:  rec.UpdatedAt,
			EventRecord: &eventRecord,
		}, true
	default:
		return runtimemodel.RuntimeEvent{}, false
	}
}

func queryEventRecordToRuntime(rec querymodel.EventRecord) runtimemodel.EventRecord {
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
