package runtime

import (
	"context"
	"time"

	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type runtimeEventStreamSink struct {
	translator streamEventTranslator
	publisher  runtimeEventPublisher
}

func newRuntimeEventStreamSink(ctx context.Context, claim runtimemodel.ClaimContext, sink kernel.EventSink) runtimeEventStreamSink {
	return runtimeEventStreamSink{
		translator: streamEventTranslator{},
		publisher: runtimeEventPublisher{
			ctx:   ctx,
			claim: claim,
			sink:  sink,
			now: func() time.Time {
				return time.Now().UTC()
			},
		},
	}
}

func (s runtimeEventStreamSink) OnStatus(string, string) {
}

func (s runtimeEventStreamSink) OnReasoningDelta(delta string) {
	event, ok := s.translator.reasoningDelta(delta)
	if !ok {
		return
	}
	s.publisher.publish(event)
}

func (s runtimeEventStreamSink) OnTextDelta(delta string) {
	event, ok := s.translator.textDelta(delta)
	if !ok {
		return
	}
	s.publisher.publish(event)
}

func (s runtimeEventStreamSink) OnToolStart(toolCallID, toolName string, args map[string]any, truncated bool) {
	s.publisher.publish(s.translator.toolStart(toolCallID, toolName, args, truncated))
}

func (s runtimeEventStreamSink) OnToolResult(toolCallID, toolName string, result map[string]any, truncated bool, apiErr *model.ErrorBlock) {
	s.publisher.publish(s.translator.toolResult(toolCallID, toolName, result, truncated, apiErr))
}

type streamEventTranslator struct{}

func (streamEventTranslator) reasoningDelta(delta string) (runtimemodel.RuntimeEvent, bool) {
	if delta == "" {
		return runtimemodel.RuntimeEvent{}, false
	}
	return runtimemodel.RuntimeEvent{
		Kind:  runtimemodel.RuntimeEventReasoningDelta,
		Delta: delta,
	}, true
}

func (streamEventTranslator) textDelta(delta string) (runtimemodel.RuntimeEvent, bool) {
	if delta == "" {
		return runtimemodel.RuntimeEvent{}, false
	}
	return runtimemodel.RuntimeEvent{
		Kind:  runtimemodel.RuntimeEventTextDelta,
		Delta: delta,
	}, true
}

func (streamEventTranslator) toolStart(toolCallID, toolName string, args map[string]any, truncated bool) runtimemodel.RuntimeEvent {
	return runtimemodel.RuntimeEvent{
		Kind:       runtimemodel.RuntimeEventToolStarted,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Args:       args,
		Truncated:  truncated,
	}
}

func (streamEventTranslator) toolResult(toolCallID, toolName string, result map[string]any, truncated bool, apiErr *model.ErrorBlock) runtimemodel.RuntimeEvent {
	return runtimemodel.RuntimeEvent{
		Kind:       runtimemodel.RuntimeEventToolFinished,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Result:     result,
		Truncated:  truncated,
		Error:      apiErr,
	}
}

type runtimeEventPublisher struct {
	ctx   context.Context
	claim runtimemodel.ClaimContext
	sink  kernel.EventSink
	now   func() time.Time
}

func (p runtimeEventPublisher) publish(event runtimemodel.RuntimeEvent) {
	if p.sink == nil {
		return
	}
	event.Work = p.claim.Work
	event.EventID = p.claim.Event.EventID
	event.RunID = p.claim.RunID
	event.SessionKey = p.claim.SessionKey
	event.SessionID = p.claim.SessionID
	if event.OccurredAt.IsZero() {
		event.OccurredAt = p.now()
	}
	_ = p.sink.Publish(p.ctx, event)
}

func nonZeroTime(in time.Time) time.Time {
	if in.IsZero() {
		return time.Now().UTC()
	}
	return in
}
