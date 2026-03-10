package store

import (
	"context"

	runnermodel "github.com/similarityyoung/simiclaw/internal/runner/model"
	"github.com/similarityyoung/simiclaw/pkg/api"
)

func (db *DB) LoadPromptHistory(ctx context.Context, sessionID string, limit int) ([]runnermodel.HistoryMessage, error) {
	items, err := db.RecentMessagesForPrompt(ctx, sessionID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]runnermodel.HistoryMessage, 0, len(items))
	for _, item := range items {
		out = append(out, runnermodel.HistoryMessage{
			Role:       item.Role,
			Content:    item.Content,
			ToolCalls:  item.ToolCalls,
			ToolCallID: item.ToolCallID,
			ToolName:   item.ToolName,
			Meta:       item.Meta,
		})
	}
	return out, nil
}

func (db *DB) SearchRAGHits(ctx context.Context, sessionID, query string, limit int) ([]runnermodel.RAGHit, error) {
	hits, err := db.SearchMessagesFTS(ctx, sessionID, query, limit)
	if err != nil {
		return nil, err
	}
	return toRunnerRAGHits(hits), nil
}

func toRunnerRAGHits(hits []api.RAGHit) []runnermodel.RAGHit {
	out := make([]runnermodel.RAGHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, runnermodel.RAGHit{
			Path:    hit.Path,
			Scope:   hit.Scope,
			Lines:   hit.Lines,
			Score:   hit.Score,
			Preview: hit.Preview,
		})
	}
	return out
}
