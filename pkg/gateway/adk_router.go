package gateway

import (
	"context"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type ADKRunResult struct {
	RunID          string
	RunMode        model.RunMode
	Entries        []model.SessionEntry
	Trace          model.RunTrace
	OutboundBody   string
	SuppressOutput bool
}

type adkSessionRouter interface {
	RouteIngest(ctx context.Context, req model.IngestRequest, sessionKey, sessionID string) error
	RunIngest(ctx context.Context, req model.IngestRequest, sessionKey, sessionID string) (ADKRunResult, error)
}
