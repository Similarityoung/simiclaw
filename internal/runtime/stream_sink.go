package runtime

import (
	"context"
	"time"

	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type runtimeEventStreamSink struct {
	ctx   context.Context
	claim runtimemodel.ClaimContext
	sink  kernel.EventSink
}

func newRuntimeEventStreamSink(ctx context.Context, claim runtimemodel.ClaimContext, sink kernel.EventSink) runtimeEventStreamSink {
	return runtimeEventStreamSink{ctx: ctx, claim: claim, sink: sink}
}

func (s runtimeEventStreamSink) OnStatus(string, string) {
}

func (s runtimeEventStreamSink) OnReasoningDelta(delta string) {
	if delta == "" {
		return
	}
	s.publish(runtimemodel.RuntimeEvent{
		Kind:  runtimemodel.RuntimeEventReasoningDelta,
		Delta: delta,
	})
}

func (s runtimeEventStreamSink) OnTextDelta(delta string) {
	if delta == "" {
		return
	}
	s.publish(runtimemodel.RuntimeEvent{
		Kind:  runtimemodel.RuntimeEventTextDelta,
		Delta: delta,
	})
}

func (s runtimeEventStreamSink) OnToolStart(toolCallID, toolName string, args map[string]any, truncated bool) {
	s.publish(runtimemodel.RuntimeEvent{
		Kind:       runtimemodel.RuntimeEventToolStarted,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Args:       args,
		Truncated:  truncated,
	})
}

func (s runtimeEventStreamSink) OnToolResult(toolCallID, toolName string, result map[string]any, truncated bool, apiErr *model.ErrorBlock) {
	s.publish(runtimemodel.RuntimeEvent{
		Kind:       runtimemodel.RuntimeEventToolFinished,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Result:     result,
		Truncated:  truncated,
		Error:      apiErr,
	})
}

func (s runtimeEventStreamSink) publish(event runtimemodel.RuntimeEvent) {
	if s.sink == nil {
		return
	}
	event.Work = s.claim.Work
	event.EventID = s.claim.Event.EventID
	event.RunID = s.claim.RunID
	event.SessionKey = s.claim.SessionKey
	event.SessionID = s.claim.SessionID
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	_ = s.sink.Publish(s.ctx, event)
}

func nonZeroTime(in time.Time) time.Time {
	if in.IsZero() {
		return time.Now().UTC()
	}
	return in
}
