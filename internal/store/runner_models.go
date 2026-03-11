package store

import (
	"context"

	runnermodel "github.com/similarityyoung/simiclaw/internal/runner/model"
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
	return db.SearchMessagesFTS(ctx, sessionID, query, limit)
}
