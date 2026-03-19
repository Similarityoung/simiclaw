package runner

import (
	"time"

	"github.com/similarityyoung/simiclaw/internal/provider"
	runnercontext "github.com/similarityyoung/simiclaw/internal/runner/context"
	runnermodel "github.com/similarityyoung/simiclaw/internal/runner/model"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type runTraceAssembler struct{}

func (runTraceAssembler) AttachContext(trace *api.RunTrace, history runnercontext.Loaded) {
	trace.ContextManifest = toAPIContextManifest(history.Manifest)
	trace.RAGHits = toAPIRAGHits(history.RAGHits)
}

func (runTraceAssembler) Fail(trace *api.RunTrace, startedAt time.Time, err error) {
	trace.Status = model.RunStatusFailed
	trace.Error = &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: err.Error()}
	trace.FinishedAt = time.Now().UTC()
	trace.LatencyMS = time.Since(startedAt).Milliseconds()
}

func (runTraceAssembler) Complete(trace *api.RunTrace, startedAt time.Time, usage provider.Usage, last provider.ChatResult, reply string) {
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

func toAPIContextManifest(in *runnermodel.ContextManifest) *api.ContextManifest {
	if in == nil {
		return nil
	}
	return &api.ContextManifest{
		HistoryRange: api.HistoryRange{
			Mode:      in.HistoryRange.Mode,
			TailLimit: in.HistoryRange.TailLimit,
		},
	}
}

func toAPIRAGHits(in []runnermodel.RAGHit) []api.RAGHit {
	out := make([]api.RAGHit, 0, len(in))
	for _, hit := range in {
		out = append(out, api.RAGHit{
			Path:    hit.Path,
			Scope:   hit.Scope,
			Lines:   hit.Lines,
			Score:   hit.Score,
			Preview: hit.Preview,
		})
	}
	return out
}
