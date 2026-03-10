package gateway

import (
	"github.com/similarityyoung/simiclaw/internal/ingest"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type Service struct {
	ingest *ingest.Service
}

type AcceptedIngest struct {
	Response   model.IngestResponse
	Result     ingest.Result
	StatusCode int
}

func NewService(ingestService *ingest.Service) *Service {
	return &Service{ingest: ingestService}
}
