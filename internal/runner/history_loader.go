package runner

import (
	"context"
	"strings"

	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type loadedHistory struct {
	history  []store.HistoryMessage
	ragHits  []model.RAGHit
	manifest *model.ContextManifest
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
	return loadedHistory{
		history: history,
		ragHits: ragHits,
		manifest: &model.ContextManifest{
			HistoryRange: model.HistoryRange{Mode: "tail", TailLimit: l.historyLimit},
		},
	}, nil
}
