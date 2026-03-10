package runner

import (
	"fmt"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type providerStreamSink struct {
	sink StreamSink
}

func (s providerStreamSink) OnReasoningDelta(delta string) {
	if s.sink != nil {
		s.sink.OnReasoningDelta(delta)
	}
}

func (s providerStreamSink) OnTextDelta(delta string) {
	if s.sink != nil {
		s.sink.OnTextDelta(delta)
	}
}

type safeStreamSink struct {
	inner StreamSink
	trace *model.RunTrace
}

func newSafeStreamSink(inner StreamSink, trace *model.RunTrace) StreamSink {
	if inner == nil {
		return noopStreamSink{}
	}
	return safeStreamSink{inner: inner, trace: trace}
}

func (s safeStreamSink) OnStatus(status, message string) {
	s.call("status", func() { s.inner.OnStatus(status, message) })
}

func (s safeStreamSink) OnReasoningDelta(delta string) {
	s.call("reasoning_delta", func() { s.inner.OnReasoningDelta(delta) })
}

func (s safeStreamSink) OnTextDelta(delta string) {
	s.call("text_delta", func() { s.inner.OnTextDelta(delta) })
}

func (s safeStreamSink) OnToolStart(toolCallID, toolName string, args map[string]any, truncated bool) {
	s.call("tool_start", func() { s.inner.OnToolStart(toolCallID, toolName, args, truncated) })
}

func (s safeStreamSink) OnToolResult(toolCallID, toolName string, result map[string]any, truncated bool, apiErr *model.ErrorBlock) {
	s.call("tool_result", func() { s.inner.OnToolResult(toolCallID, toolName, result, truncated, apiErr) })
}

func (s safeStreamSink) call(kind string, fn func()) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if s.trace.Diagnostics == nil {
				s.trace.Diagnostics = map[string]string{}
			}
			s.trace.Diagnostics["stream_sink."+kind] = fmt.Sprintf("panic: %v", recovered)
		}
	}()
	fn()
}

type noopStreamSink struct{}

func (noopStreamSink) OnStatus(string, string) {}

func (noopStreamSink) OnReasoningDelta(string) {}

func (noopStreamSink) OnTextDelta(string) {}

func (noopStreamSink) OnToolStart(string, string, map[string]any, bool) {}

func (noopStreamSink) OnToolResult(string, string, map[string]any, bool, *model.ErrorBlock) {}
