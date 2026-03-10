package runner

import (
	"context"
	"strings"

	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/api"
)

type loadedHistory struct {
	history  []store.HistoryMessage
	ragHits  []api.RAGHit
	manifest *api.ContextManifest
}

type runHistoryLoader struct {
	reader       HistoryReader
	historyLimit int
}

func (l runHistoryLoader) Load(ctx context.Context, sessionID, query string) (loadedHistory, error) {
	history, err := l.reader.RecentMessagesForPrompt(ctx, sessionID, l.historyLimit)
	if err != nil {
		return loadedHistory{}, err
	}
	ragHits, _ := l.reader.SearchMessagesFTS(ctx, sessionID, strings.TrimSpace(query), 5)
	apiHits := make([]api.RAGHit, 0, len(ragHits))
	for _, hit := range ragHits {
		apiHits = append(apiHits, api.RAGHit{
			Path:    hit.Path,
			Scope:   hit.Scope,
			Lines:   hit.Lines,
			Score:   hit.Score,
			Preview: hit.Preview,
		})
	}
	return loadedHistory{
		history: history,
		ragHits: apiHits,
		manifest: &api.ContextManifest{
			HistoryRange: api.HistoryRange{Mode: "tail", TailLimit: l.historyLimit},
		},
	}, nil
}
