package runtime

import (
	"github.com/similarityyoung/simiclaw/pkg/api"
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

func terminalEventFromRecord(rec model.EventRecord) api.ChatStreamEvent {
	switch rec.Status {
	case model.EventStatusFailed:
		return api.ChatStreamEvent{
			Type:        api.ChatStreamEventError,
			EventID:     rec.EventID,
			At:          nonZeroTime(rec.UpdatedAt),
			EventRecord: &rec,
			Error:       rec.Error,
		}
	default:
		return api.ChatStreamEvent{
			Type:        api.ChatStreamEventDone,
			EventID:     rec.EventID,
			At:          nonZeroTime(rec.UpdatedAt),
			EventRecord: &rec,
		}
	}
}

func terminalEventFromFinalize(finalize store.RunFinalize) api.ChatStreamEvent {
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
