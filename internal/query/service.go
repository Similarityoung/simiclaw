package query

import (
	"context"

	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
)

type Repository interface {
	GetEventRecord(ctx context.Context, eventID string) (querymodel.EventRecord, bool, error)
	LookupEvent(ctx context.Context, key string) (querymodel.LookupEvent, bool, error)
	GetRunTrace(ctx context.Context, runID string) (querymodel.RunTrace, bool, error)
	GetSessionRecord(ctx context.Context, sessionKey string) (querymodel.SessionRecord, bool, error)
	ListMessageRecords(ctx context.Context, filter querymodel.SessionHistoryFilter) ([]querymodel.MessageRecord, error)
	ListEventRecords(ctx context.Context, filter querymodel.EventFilter) ([]querymodel.EventRecord, error)
	ListRunTraces(ctx context.Context, filter querymodel.RunFilter) ([]querymodel.RunTrace, error)
	ListSessionRecords(ctx context.Context, filter querymodel.SessionFilter) ([]querymodel.SessionRecord, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func pageFetchLimit(limit int) int {
	if limit <= 0 {
		return 51
	}
	return limit + 1
}
