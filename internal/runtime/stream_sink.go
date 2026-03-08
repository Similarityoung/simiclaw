package runtime

import (
	"time"

	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/internal/streaming"
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
	s.hub.Publish(s.eventID, model.ChatStreamEvent{
		Type:    model.ChatStreamEventStatus,
		Status:  status,
		Message: message,
	})
}

func (s hubStreamSink) OnReasoningDelta(delta string) {
	if s.hub == nil || delta == "" {
		return
	}
	s.hub.Publish(s.eventID, model.ChatStreamEvent{
		Type:  model.ChatStreamEventReasoningDelta,
		Delta: delta,
	})
}

func (s hubStreamSink) OnTextDelta(delta string) {
	if s.hub == nil || delta == "" {
		return
	}
	s.hub.Publish(s.eventID, model.ChatStreamEvent{
		Type:  model.ChatStreamEventTextDelta,
		Delta: delta,
	})
}

func (s hubStreamSink) OnToolStart(toolCallID, toolName string, args map[string]any, truncated bool) {
	if s.hub == nil {
		return
	}
	s.hub.Publish(s.eventID, model.ChatStreamEvent{
		Type:       model.ChatStreamEventToolStart,
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
	s.hub.Publish(s.eventID, model.ChatStreamEvent{
		Type:       model.ChatStreamEventToolResult,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Result:     result,
		Truncated:  truncated,
		Error:      apiErr,
	})
}

func terminalEventFromRecord(rec model.EventRecord) model.ChatStreamEvent {
	switch rec.Status {
	case model.EventStatusFailed:
		return model.ChatStreamEvent{
			Type:        model.ChatStreamEventError,
			EventID:     rec.EventID,
			At:          nonZeroTime(rec.UpdatedAt),
			EventRecord: &rec,
			Error:       rec.Error,
		}
	default:
		return model.ChatStreamEvent{
			Type:        model.ChatStreamEventDone,
			EventID:     rec.EventID,
			At:          nonZeroTime(rec.UpdatedAt),
			EventRecord: &rec,
		}
	}
}

func terminalEventFromFinalize(finalize store.RunFinalize) model.ChatStreamEvent {
	rec := model.EventRecord{
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
