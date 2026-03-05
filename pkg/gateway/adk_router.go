package gateway

import (
	"context"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type adkSessionRouter interface {
	RouteIngest(ctx context.Context, req model.IngestRequest, sessionKey, sessionID string) error
}
