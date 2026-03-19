package context

import (
	"context"

	runnermodel "github.com/similarityyoung/simiclaw/internal/runner/model"
)

type Reader interface {
	LoadPromptHistory(ctx context.Context, sessionID string, limit int) ([]runnermodel.HistoryMessage, error)
	SearchRAGHits(ctx context.Context, sessionID, query string, limit int) ([]runnermodel.RAGHit, error)
}

type Loaded struct {
	History  []runnermodel.HistoryMessage
	RAGHits  []runnermodel.RAGHit
	Manifest *runnermodel.ContextManifest
}
