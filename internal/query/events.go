package query

import (
	"context"

	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
)

func (s *Service) GetEvent(ctx context.Context, eventID string) (querymodel.EventRecord, bool, error) {
	return s.repo.GetEventRecord(ctx, eventID)
}

func (s *Service) LookupEvent(ctx context.Context, idempotencyKey string) (querymodel.LookupEvent, bool, error) {
	return s.repo.LookupEvent(ctx, idempotencyKey)
}

func (s *Service) ListEvents(ctx context.Context, filter querymodel.EventFilter) (querymodel.EventPage, error) {
	filter.Limit = pageFetchLimit(filter.Limit)
	items, err := s.repo.ListEventRecords(ctx, filter)
	if err != nil {
		return querymodel.EventPage{}, err
	}
	return buildEventPage(items, filter.Limit-1), nil
}

func buildEventPage(items []querymodel.EventRecord, limit int) querymodel.EventPage {
	if limit <= 0 || len(items) <= limit {
		return querymodel.EventPage{Items: items}
	}
	trimmed := items[:limit]
	last := trimmed[len(trimmed)-1]
	return querymodel.EventPage{
		Items: trimmed,
		Next: &querymodel.EventCursorAnchor{
			CreatedAt: last.CreatedAt,
			EventID:   last.EventID,
		},
	}
}
