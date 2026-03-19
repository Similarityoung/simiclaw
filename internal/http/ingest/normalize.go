package ingest

import (
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func NormalizeAPIRequest(req api.IngestRequest) (gatewaymodel.NormalizedIngress, *gateway.APIError) {
	if strings.TrimSpace(req.Timestamp) == "" {
		return gatewaymodel.NormalizedIngress{}, &gateway.APIError{
			StatusCode: 400,
			Code:       model.ErrorCodeInvalidArgument,
			Message:    "field timestamp is required",
			Details:    map[string]any{"field": "timestamp"},
		}
	}
	if !strings.HasSuffix(req.Timestamp, "Z") {
		return gatewaymodel.NormalizedIngress{}, &gateway.APIError{
			StatusCode: 400,
			Code:       model.ErrorCodeInvalidArgument,
			Message:    "timestamp must be UTC",
		}
	}
	ts, err := time.Parse(time.RFC3339Nano, req.Timestamp)
	if err != nil {
		return gatewaymodel.NormalizedIngress{}, &gateway.APIError{
			StatusCode: 400,
			Code:       model.ErrorCodeInvalidArgument,
			Message:    "invalid timestamp",
		}
	}
	return gatewaymodel.NormalizedIngress{
		Source:         req.Source,
		Conversation:   req.Conversation,
		SessionKeyHint: req.SessionKeyHint,
		IdempotencyKey: req.IdempotencyKey,
		Timestamp:      ts.UTC(),
		DMScope:        req.DMScope,
		Payload:        req.Payload,
	}, nil
}
