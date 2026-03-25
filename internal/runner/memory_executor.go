package runner

import (
	"context"
	"strings"
	"time"

	runnercontext "github.com/similarityyoung/simiclaw/internal/runner/context"
	runtimepayload "github.com/similarityyoung/simiclaw/internal/runtime/payload"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type memoryRunExecutor struct {
	writer runnercontext.MemoryWriter
}

func (e memoryRunExecutor) Execute(ctx context.Context, event model.InternalEvent, now time.Time, trace *api.RunTrace, plan runtimepayload.Plan) (RunOutput, error) {
	note := strings.TrimSpace(event.Payload.Text)
	if note == "" {
		note = event.Payload.Type
	}
	switch plan.MemoryWriteTarget {
	case runtimepayload.MemoryWriteTargetDaily:
		_ = e.writer.WriteDaily("system:"+event.Payload.Type, note, now, event.Conversation.ChannelType)
	case runtimepayload.MemoryWriteTargetCurated:
		_ = e.writer.WriteCurated(note, now, event.Conversation.ChannelType)
	}
	trace.OutputText = note
	trace.FinishedAt = time.Now().UTC()
	trace.LatencyMS = time.Since(now).Milliseconds()
	runLogger(ctx, "runner", event).Info(
		"memory write completed",
		logging.String("run_mode", string(plan.RunMode)),
		logging.String("memory_write_target", string(plan.MemoryWriteTarget)),
		logging.Int64("latency_ms", trace.LatencyMS),
	)
	return RunOutput{
		RunMode: plan.RunMode,
		Messages: []OutputMessage{{
			Role:    "system",
			Content: note,
			Visible: false,
			Meta:    cloneMap(plan.MessageMeta),
		}},
		Trace:          *trace,
		AssistantReply: "",
		SuppressOutput: plan.SuppressOutput,
	}, nil
}
