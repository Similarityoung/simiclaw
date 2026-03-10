package gateway

import (
	"context"
	"fmt"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"net/http"
	"time"

	"github.com/similarityyoung/simiclaw/internal/ingest"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (s *Service) Ingest(ctx context.Context, req api.IngestRequest) (api.IngestResponse, int, *APIError) {
	accepted, apiErr := s.Accept(ctx, req)
	if apiErr != nil {
		return api.IngestResponse{}, 0, apiErr
	}
	return accepted.Response, accepted.StatusCode, nil
}

func (s *Service) Accept(ctx context.Context, req api.IngestRequest) (AcceptedIngest, *APIError) {
	if s.ingest == nil {
		return AcceptedIngest{}, &APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: "ingest service unavailable"}
	}
	result, err := s.ingest.Ingest(ctx, ingest.Command{Request: req})
	if result.Duplicate {
		return AcceptedIngest{
			Response: api.IngestResponse{
				EventID:         result.EventID,
				SessionKey:      result.SessionKey,
				ActiveSessionID: result.SessionID,
				ReceivedAt:      result.ReceivedAt.Format(time.RFC3339Nano),
				PayloadHash:     result.PayloadHash,
				Status:          ingestStatusDuplicate,
				StatusURL:       fmt.Sprintf(ingestStatusURLTemplate, result.EventID),
			},
			Result:     result,
			StatusCode: http.StatusOK,
		}, nil
	}
	if err != nil {
		return AcceptedIngest{}, mapIngestError(err)
	}
	return AcceptedIngest{
		Response: api.IngestResponse{
			EventID:         result.EventID,
			SessionKey:      result.SessionKey,
			ActiveSessionID: result.SessionID,
			ReceivedAt:      result.ReceivedAt.Format(time.RFC3339Nano),
			PayloadHash:     result.PayloadHash,
			Status:          ingestStatusAccepted,
			StatusURL:       fmt.Sprintf(ingestStatusURLTemplate, result.EventID),
		},
		Result:     result,
		StatusCode: http.StatusAccepted,
	}, nil
}

func mapIngestError(err *ingest.Error) *APIError {
	if err == nil {
		return nil
	}
	statusCode := http.StatusInternalServerError
	switch err.Code {
	case model.ErrorCodeInvalidArgument:
		statusCode = http.StatusBadRequest
	case model.ErrorCodeConflict:
		statusCode = http.StatusConflict
	case model.ErrorCodeRateLimited:
		statusCode = http.StatusTooManyRequests
	}
	return &APIError{
		StatusCode: statusCode,
		Code:       err.Code,
		Message:    err.Message,
		Details:    err.Details,
		RetryAfter: err.RetryAfter,
	}
}
