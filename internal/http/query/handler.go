package query

import (
	"context"

	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
)

type Query interface {
	GetEvent(ctx context.Context, eventID string) (querymodel.EventRecord, bool, error)
	LookupEvent(ctx context.Context, idempotencyKey string) (querymodel.LookupEvent, bool, error)
	ListEvents(ctx context.Context, filter querymodel.EventFilter) (querymodel.EventPage, error)
	ListRuns(ctx context.Context, filter querymodel.RunFilter) (querymodel.RunPage, error)
	GetRun(ctx context.Context, runID string) (querymodel.RunTrace, bool, error)
	ListSessions(ctx context.Context, filter querymodel.SessionFilter) (querymodel.SessionPage, error)
	GetSession(ctx context.Context, sessionKey string) (querymodel.SessionRecord, bool, error)
	ListSessionHistory(ctx context.Context, filter querymodel.SessionHistoryFilter) (querymodel.MessagePage, error)
}

type Handlers struct {
	query Query
}

func NewHandlers(query Query) *Handlers {
	return &Handlers{query: query}
}
