package gateway

import (
	"context"

	"github.com/similarityyoung/simiclaw/internal/ingest"
	"github.com/similarityyoung/simiclaw/pkg/api"
)

type Ingestor interface {
	Ingest(ctx context.Context, cmd ingest.Command) (ingest.Result, *ingest.Error)
}

type Service struct {
	ingest Ingestor
}

type AcceptedIngest struct {
	Response   api.IngestResponse
	Result     ingest.Result
	StatusCode int
}

func NewService(ingestService Ingestor) *Service {
	return &Service{ingest: ingestService}
}
