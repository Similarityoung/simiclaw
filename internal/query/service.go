package query

import (
	"context"

	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
)

type EventRepository interface {
	GetEventRecord(ctx context.Context, eventID string) (querymodel.EventRecord, bool, error)
	LookupEvent(ctx context.Context, key string) (querymodel.LookupEvent, bool, error)
	ListEventRecords(ctx context.Context, filter querymodel.EventFilter) ([]querymodel.EventRecord, error)
}

type RunRepository interface {
	GetRunTrace(ctx context.Context, runID string) (querymodel.RunTrace, bool, error)
	ListRunTraces(ctx context.Context, filter querymodel.RunFilter) ([]querymodel.RunTrace, error)
}

type SessionRepository interface {
	GetSessionRecord(ctx context.Context, sessionKey string) (querymodel.SessionRecord, bool, error)
	ListMessageRecords(ctx context.Context, filter querymodel.SessionHistoryFilter) ([]querymodel.MessageRecord, error)
	ListSessionRecords(ctx context.Context, filter querymodel.SessionFilter) ([]querymodel.SessionRecord, error)
}

type Service struct {
	events   EventRepository
	runs     RunRepository
	sessions SessionRepository
}

func NewService(events EventRepository, runs RunRepository, sessions SessionRepository) *Service {
	return &Service{
		events:   events,
		runs:     runs,
		sessions: sessions,
	}
}

func pageFetchLimit(limit int) int {
	if limit <= 0 {
		return 51
	}
	return limit + 1
}
