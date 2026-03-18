package runtime

import (
	"context"

	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/internal/streaming"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type hubRuntimeEventSink struct {
	hub   *streaming.Hub
	facts kernel.Facts
}

func newHubRuntimeEventSink(hub *streaming.Hub, facts kernel.Facts) kernel.EventSink {
	return hubRuntimeEventSink{hub: hub, facts: facts}
}

func (s hubRuntimeEventSink) Publish(ctx context.Context, event runtimemodel.RuntimeEvent) error {
	if s.hub == nil || event.EventID == "" {
		return nil
	}
	switch event.Kind {
	case runtimemodel.RuntimeEventClaimed:
		s.hub.Publish(event.EventID, api.ChatStreamEvent{
			Type:    api.ChatStreamEventStatus,
			Status:  "processing",
			Message: "claimed",
			At:      nonZeroTime(event.OccurredAt),
		})
	case runtimemodel.RuntimeEventExecuting:
		s.hub.Publish(event.EventID, api.ChatStreamEvent{
			Type:    api.ChatStreamEventStatus,
			Status:  "processing",
			Message: "running",
			At:      nonZeroTime(event.OccurredAt),
		})
	case runtimemodel.RuntimeEventFinalizeStarted:
		s.hub.Publish(event.EventID, api.ChatStreamEvent{
			Type:    api.ChatStreamEventStatus,
			Status:  "processing",
			Message: "finalizing",
			At:      nonZeroTime(event.OccurredAt),
		})
	case runtimemodel.RuntimeEventCompleted, runtimemodel.RuntimeEventFailed:
		if s.facts != nil {
			if rec, ok, err := s.facts.GetEventRecord(ctx, event.EventID); err == nil && ok {
				s.hub.PublishTerminal(event.EventID, terminalEventFromRecord(rec))
				return nil
			}
		}
		if event.Kind == runtimemodel.RuntimeEventFailed {
			s.hub.PublishTerminal(event.EventID, api.ChatStreamEvent{
				Type:  api.ChatStreamEventError,
				At:    nonZeroTime(event.OccurredAt),
				Error: event.Error,
			})
			return nil
		}
		s.hub.PublishTerminal(event.EventID, api.ChatStreamEvent{
			Type: api.ChatStreamEventDone,
			At:   nonZeroTime(event.OccurredAt),
			EventRecord: &api.EventRecord{
				EventID:    event.EventID,
				SessionKey: event.SessionKey,
				SessionID:  event.SessionID,
				RunID:      event.RunID,
				Status:     model.EventStatusProcessed,
			},
		})
	}
	return nil
}
