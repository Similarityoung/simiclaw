package query

import (
	"context"

	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (s *Service) GetRun(ctx context.Context, runID string) (model.RunTrace, bool, error) {
	return s.repo.GetRun(ctx, runID)
}

func (s *Service) ListRuns(ctx context.Context, query RunListQuery) (RunPage, error) {
	filter := store.RunListFilter{
		SessionKey: query.SessionKey,
		SessionID:  query.SessionID,
		Limit:      pageFetchLimit(query.Limit),
	}
	if query.Cursor != nil {
		filter.CursorStartedAt = query.Cursor.StartedAt
		filter.CursorRunID = query.Cursor.RunID
	}
	items, err := s.repo.ListRunsPage(ctx, filter)
	if err != nil {
		return RunPage{}, err
	}
	return buildRunPage(items, query.Limit), nil
}

func buildRunPage(items []model.RunTrace, limit int) RunPage {
	if limit <= 0 || len(items) <= limit {
		return RunPage{Items: items}
	}
	trimmed := items[:limit]
	last := trimmed[len(trimmed)-1]
	return RunPage{
		Items: trimmed,
		Next: &RunCursorAnchor{
			StartedAt: last.StartedAt,
			RunID:     last.RunID,
		},
	}
}
