package runner

import (
	"context"
	"strings"

	runnermodel "github.com/similarityyoung/simiclaw/internal/runner/model"
)

type loadedHistory struct {
	history  []runnermodel.HistoryMessage
	ragHits  []runnermodel.RAGHit
	manifest *runnermodel.ContextManifest
}

type runHistoryLoader struct {
	reader       HistoryReader
	historyLimit int
}

func (l runHistoryLoader) Load(ctx context.Context, sessionID, query string) (loadedHistory, error) {
	history, err := l.reader.LoadPromptHistory(ctx, sessionID, l.historyLimit)
	if err != nil {
		return loadedHistory{}, err
	}
	ragHits, _ := l.reader.SearchRAGHits(ctx, sessionID, strings.TrimSpace(query), 5)
	return loadedHistory{
		history: history,
		ragHits: ragHits,
		manifest: &runnermodel.ContextManifest{
			HistoryRange: runnermodel.HistoryRange{Mode: "tail", TailLimit: l.historyLimit},
		},
	}, nil
}
