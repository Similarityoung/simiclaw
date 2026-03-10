package gateway

import (
	"github.com/similarityyoung/simiclaw/internal/ingest"
	"github.com/similarityyoung/simiclaw/pkg/api"
)

type Service struct {
	ingest *ingest.Service
}

type AcceptedIngest struct {
	Response   api.IngestResponse
	Result     ingest.Result
	StatusCode int
}

func NewService(ingestService *ingest.Service) *Service {
	return &Service{ingest: ingestService}
}
