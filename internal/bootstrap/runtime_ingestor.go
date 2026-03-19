package bootstrap

import (
	"context"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
	runtimeworkers "github.com/similarityyoung/simiclaw/internal/runtime/workers"
)

type runtimeEventIngestor struct {
	gateway *gateway.Service
}

func newRuntimeEventIngestor(gatewayService *gateway.Service) runtimeworkers.EventIngestor {
	if gatewayService == nil {
		return nil
	}
	return runtimeEventIngestor{gateway: gatewayService}
}

func (i runtimeEventIngestor) Ingest(ctx context.Context, req runtimeworkers.IngestRequest) (runtimeworkers.IngestResult, error) {
	accepted, apiErr := i.gateway.Accept(ctx, gatewaymodel.NormalizedIngress{
		Source:         req.Source,
		Conversation:   req.Conversation,
		IdempotencyKey: req.IdempotencyKey,
		Timestamp:      req.Timestamp,
		Payload:        req.Payload,
	})
	if apiErr != nil {
		return runtimeworkers.IngestResult{}, apiErr
	}
	return runtimeworkers.IngestResult{
		EventID:   accepted.Result.EventID,
		Duplicate: accepted.Result.Duplicate,
		Enqueued:  accepted.Result.Enqueued,
	}, nil
}
