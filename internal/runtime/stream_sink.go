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
		publisher:  newRuntimeEventPublisher(ctx, claim, sink),
	}
}

func (s runtimeEventStreamSink) OnStatus(string, string) {
}

func (s runtimeEventStreamSink) OnReasoningDelta(delta string) {
	s.publisher.publishTranslated(s.translator.reasoningDelta(delta))
}

func (s runtimeEventStreamSink) OnTextDelta(delta string) {
	s.publisher.publishTranslated(s.translator.textDelta(delta))
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

func newRuntimeEventPublisher(ctx context.Context, claim runtimemodel.ClaimContext, sink kernel.EventSink) runtimeEventPublisher {
	return runtimeEventPublisher{
		ctx:   ctx,
		claim: claim,
		sink:  sink,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (p runtimeEventPublisher) publishTranslated(event runtimemodel.RuntimeEvent, ok bool) {
	if !ok {
		return
	}
	p.publish(event)
}

func (p runtimeEventPublisher) publish(event runtimemodel.RuntimeEvent) {
	if p.sink == nil {
		return
	}
	_ = p.sink.Publish(p.ctx, p.populateMetadata(event))
}

func (p runtimeEventPublisher) populateMetadata(event runtimemodel.RuntimeEvent) runtimemodel.RuntimeEvent {
	event.Work = p.claim.Work
	event.EventID = p.claim.Event.EventID
	event.RunID = p.claim.RunID
	event.SessionKey = p.claim.SessionKey
	event.SessionID = p.claim.SessionID
	if event.OccurredAt.IsZero() {
		event.OccurredAt = p.now()
	}
	return event
}

func nonZeroTime(in time.Time) time.Time {
	if in.IsZero() {
		return time.Now().UTC()
	}
	return in
}
