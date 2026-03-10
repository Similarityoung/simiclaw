package query

import (
	"context"

	"github.com/similarityyoung/simiclaw/internal/readmodel"
	"github.com/similarityyoung/simiclaw/internal/store"
)

func (s *Service) GetEvent(ctx context.Context, eventID string) (readmodel.EventRecord, bool, error) {
	return s.repo.GetEvent(ctx, eventID)
}

func (s *Service) LookupEvent(ctx context.Context, idempotencyKey string) (readmodel.LookupEvent, bool, error) {
	return s.repo.LookupInbound(ctx, idempotencyKey)
}

func (s *Service) ListEvents(ctx context.Context, query EventListQuery) (EventPage, error) {
	filter := store.EventListFilter{
		SessionKey: query.SessionKey,
		Status:     query.Status,
		Limit:      pageFetchLimit(query.Limit),
	}
	if query.Cursor != nil {
		filter.CursorCreatedAt = query.Cursor.CreatedAt
		filter.CursorEventID = query.Cursor.EventID
	}
	items, err := s.repo.ListEventsPage(ctx, filter)
	if err != nil {
		return EventPage{}, err
	}
	return buildEventPage(items, query.Limit), nil
}

func buildEventPage(items []readmodel.EventRecord, limit int) EventPage {
	if limit <= 0 || len(items) <= limit {
		return EventPage{Items: items}
	}
	trimmed := items[:limit]
	last := trimmed[len(trimmed)-1]
	return EventPage{
		Items: trimmed,
		Next: &EventCursorAnchor{
			CreatedAt: last.CreatedAt,
			EventID:   last.EventID,
		},
	}
}
