package runner

import (
	"time"

	"github.com/similarityyoung/simiclaw/internal/provider"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type runTraceAssembler struct{}

func (runTraceAssembler) AttachContext(trace *model.RunTrace, history loadedHistory) {
	trace.ContextManifest = history.manifest
	trace.RAGHits = history.ragHits
}

func (runTraceAssembler) Fail(trace *model.RunTrace, startedAt time.Time, err error) {
	trace.Status = model.RunStatusFailed
	trace.Error = &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: err.Error()}
	trace.FinishedAt = time.Now().UTC()
	trace.LatencyMS = time.Since(startedAt).Milliseconds()
}

func (runTraceAssembler) Complete(trace *model.RunTrace, startedAt time.Time, usage provider.Usage, last provider.ChatResult, reply string) {
	trace.Provider = last.Provider
	trace.Model = last.Model
	trace.PromptTokens = usage.PromptTokens
	trace.CompletionTokens = usage.CompletionTokens
	trace.TotalTokens = usage.TotalTokens
	trace.FinishReason = last.FinishReason
	trace.RawFinishReason = last.RawFinishReason
	trace.ProviderRequestID = last.ProviderRequestID
	trace.OutputText = reply
	trace.ToolCalls = last.ToolCalls
	trace.FinishedAt = time.Now().UTC()
	trace.LatencyMS = time.Since(startedAt).Milliseconds()
}
