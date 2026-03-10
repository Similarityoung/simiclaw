package runtime

import (
	"time"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/internal/streaming"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type hubStreamSink struct {
	hub     *streaming.Hub
	eventID string
}

func newHubStreamSink(hub *streaming.Hub, eventID string) hubStreamSink {
	return hubStreamSink{hub: hub, eventID: eventID}
}

func (s hubStreamSink) OnStatus(status, message string) {
	if s.hub == nil {
		return
	}
	s.hub.Publish(s.eventID, api.ChatStreamEvent{
		Type:    api.ChatStreamEventStatus,
		Status:  status,
		Message: message,
	})
}

func (s hubStreamSink) OnReasoningDelta(delta string) {
	if s.hub == nil || delta == "" {
		return
	}
	s.hub.Publish(s.eventID, api.ChatStreamEvent{
		Type:  api.ChatStreamEventReasoningDelta,
		Delta: delta,
	})
}

func (s hubStreamSink) OnTextDelta(delta string) {
	if s.hub == nil || delta == "" {
		return
	}
	s.hub.Publish(s.eventID, api.ChatStreamEvent{
		Type:  api.ChatStreamEventTextDelta,
		Delta: delta,
	})
}

func (s hubStreamSink) OnToolStart(toolCallID, toolName string, args map[string]any, truncated bool) {
	if s.hub == nil {
		return
	}
	s.hub.Publish(s.eventID, api.ChatStreamEvent{
		Type:       api.ChatStreamEventToolStart,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Args:       args,
		Truncated:  truncated,
	})
}

func (s hubStreamSink) OnToolResult(toolCallID, toolName string, result map[string]any, truncated bool, apiErr *model.ErrorBlock) {
	if s.hub == nil {
		return
	}
	s.hub.Publish(s.eventID, api.ChatStreamEvent{
		Type:       api.ChatStreamEventToolResult,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Result:     result,
		Truncated:  truncated,
		Error:      apiErr,
	})
}

func terminalEventFromRecord(rec runtimemodel.EventRecord) api.ChatStreamEvent {
	apiRec := api.EventRecord{
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
	switch rec.Status {
	case model.EventStatusFailed:
		return api.ChatStreamEvent{
			Type:        api.ChatStreamEventError,
			EventID:     rec.EventID,
			At:          nonZeroTime(rec.UpdatedAt),
			EventRecord: &apiRec,
			Error:       rec.Error,
		}
	default:
		return api.ChatStreamEvent{
			Type:        api.ChatStreamEventDone,
			EventID:     rec.EventID,
			At:          nonZeroTime(rec.UpdatedAt),
			EventRecord: &apiRec,
		}
	}
}

func terminalEventFromFinalize(finalize runtimemodel.RunFinalize) api.ChatStreamEvent {
	rec := runtimemodel.EventRecord{
		EventID:        finalize.EventID,
		Status:         finalize.EventStatus,
		RunID:          finalize.RunID,
		RunMode:        finalize.RunMode,
		SessionKey:     finalize.SessionKey,
		SessionID:      finalize.SessionID,
		AssistantReply: finalize.AssistantReply,
		Provider:       finalize.Provider,
		Model:          finalize.Model,
		UpdatedAt:      nonZeroTime(finalize.Now),
		Error:          finalize.Error,
	}
	return terminalEventFromRecord(rec)
}

func nonZeroTime(in time.Time) time.Time {
	if in.IsZero() {
		return time.Now().UTC()
	}
	return in
}
