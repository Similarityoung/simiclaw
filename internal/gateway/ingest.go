package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/similarityyoung/simiclaw/internal/session"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (s *Service) Ingest(ctx context.Context, req model.IngestRequest) (model.IngestResponse, int, *APIError) {
	accepted, apiErr := s.Accept(ctx, req)
	if apiErr != nil {
		return model.IngestResponse{}, 0, apiErr
	}
	return accepted.Response, accepted.StatusCode, nil
}

func (s *Service) Accept(ctx context.Context, req model.IngestRequest) (AcceptedIngest, *APIError) {
	start := time.Now().UTC()
	logger := logging.L("gateway").With(
		logging.String("tenant_id", s.cfg.TenantID),
		logging.String("conversation_id", req.Conversation.ConversationID),
	)

	ts, apiErr := validateRequest(req, start)
	if apiErr != nil {
		return AcceptedIngest{}, apiErr
	}

	sessionKey, err := session.ComputeKey(s.cfg.TenantID, req.Conversation, "default")
	if err != nil {
		return AcceptedIngest{}, &APIError{
			StatusCode: http.StatusBadRequest,
			Code:       model.ErrorCodeInvalidArgument,
			Message:    err.Error(),
		}
	}
	if !s.tenantLimiter.Allow(s.cfg.TenantID, start) || !s.sessionLimiter.Allow(sessionKey, start) {
		return AcceptedIngest{}, &APIError{
			StatusCode: http.StatusTooManyRequests,
			Code:       model.ErrorCodeRateLimited,
			Message:    "rate limited",
			RetryAfter: retryAfterSeconds,
		}
	}

	payloadHash, err := canonicalPayloadHash(req)
	if err != nil {
		return AcceptedIngest{}, &APIError{
			StatusCode: http.StatusBadRequest,
			Code:       model.ErrorCodeInvalidArgument,
			Message:    "invalid payload",
		}
	}

	ingestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := s.db.IngestEvent(ingestCtx, s.cfg.TenantID, sessionKey, req, payloadHash, ts)
	if err != nil {
		if errors.Is(err, store.ErrIdempotencyConflict) {
			return AcceptedIngest{}, &APIError{
				StatusCode: http.StatusConflict,
				Code:       model.ErrorCodeConflict,
				Message:    msgIdempotencyConflict,
			}
		}
		return AcceptedIngest{}, &APIError{
			StatusCode: http.StatusInternalServerError,
			Code:       model.ErrorCodeInternal,
			Message:    err.Error(),
		}
	}
	if result.Duplicate {
		return AcceptedIngest{
			Response: model.IngestResponse{
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

	if s.eventLoop.TryEnqueue(result.EventID) {
		_ = s.db.MarkEventQueued(ctx, result.EventID, time.Now().UTC())
	}
	logger.Info("gateway.ingest.accepted",
		logging.String("status", ingestStatusAccepted),
		logging.String("event_id", result.EventID),
		logging.String("session_key", sessionKey),
		logging.Int64("latency_ms", time.Since(start).Milliseconds()),
	)
	return AcceptedIngest{
		Response: model.IngestResponse{
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
