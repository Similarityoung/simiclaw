package query

import (
	"context"
	"time"

	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type EventCursorAnchor struct {
	CreatedAt time.Time
	EventID   string
}

type RunCursorAnchor struct {
	StartedAt time.Time
	RunID     string
}

type SessionCursorAnchor struct {
	LastActivityAt time.Time
	SessionKey     string
}

type EventListQuery struct {
	SessionKey string
	Status     model.EventStatus
	Limit      int
	Cursor     *EventCursorAnchor
}

type RunListQuery struct {
	SessionKey string
	SessionID  string
	Limit      int
	Cursor     *RunCursorAnchor
}

type SessionListQuery struct {
	SessionKey     string
	ConversationID string
	Limit          int
	Cursor         *SessionCursorAnchor
}

type EventPage struct {
	Items []model.EventRecord
	Next  *EventCursorAnchor
}

type RunPage struct {
	Items []model.RunTrace
	Next  *RunCursorAnchor
}

type SessionPage struct {
	Items []model.SessionRecord
	Next  *SessionCursorAnchor
}

type Repository interface {
	ListEventsPage(ctx context.Context, filter store.EventListFilter) ([]model.EventRecord, error)
	ListRunsPage(ctx context.Context, filter store.RunListFilter) ([]model.RunTrace, error)
	ListSessionsPage(ctx context.Context, filter store.SessionListFilter) ([]model.SessionRecord, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
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

func (s *Service) ListSessions(ctx context.Context, query SessionListQuery) (SessionPage, error) {
	filter := store.SessionListFilter{
		SessionKey:     query.SessionKey,
		ConversationID: query.ConversationID,
		Limit:          pageFetchLimit(query.Limit),
	}
	if query.Cursor != nil {
		filter.CursorLastActivityAt = query.Cursor.LastActivityAt
		filter.CursorLastSessionKey = query.Cursor.SessionKey
	}
	items, err := s.repo.ListSessionsPage(ctx, filter)
	if err != nil {
		return SessionPage{}, err
	}
	return buildSessionPage(items, query.Limit), nil
}

func buildEventPage(items []model.EventRecord, limit int) EventPage {
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

func buildSessionPage(items []model.SessionRecord, limit int) SessionPage {
	if limit <= 0 || len(items) <= limit {
		return SessionPage{Items: items}
	}
	trimmed := items[:limit]
	last := trimmed[len(trimmed)-1]
	return SessionPage{
		Items: trimmed,
		Next: &SessionCursorAnchor{
			LastActivityAt: last.LastActivityAt,
			SessionKey:     last.SessionKey,
		},
	}
}

func pageFetchLimit(limit int) int {
	if limit <= 0 {
		return 51
	}
	return limit + 1
}
