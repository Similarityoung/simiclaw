package query

import (
	"context"

	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
)

func (s *Service) GetRun(ctx context.Context, runID string) (querymodel.RunTrace, bool, error) {
	return s.repo.GetRunTrace(ctx, runID)
}

func (s *Service) ListRuns(ctx context.Context, filter querymodel.RunFilter) (querymodel.RunPage, error) {
	filter.Limit = pageFetchLimit(filter.Limit)
	items, err := s.repo.ListRunTraces(ctx, filter)
	if err != nil {
		return querymodel.RunPage{}, err
	}
	return buildRunPage(items, filter.Limit-1), nil
}

func buildRunPage(items []querymodel.RunTrace, limit int) querymodel.RunPage {
	if limit <= 0 || len(items) <= limit {
		return querymodel.RunPage{Items: items}
	}
	trimmed := items[:limit]
	last := trimmed[len(trimmed)-1]
	return querymodel.RunPage{
		Items: trimmed,
		Next: &querymodel.RunCursorAnchor{
			StartedAt: last.StartedAt,
			RunID:     last.RunID,
		},
	}
}
