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
	start := time.Now().UTC()
	logger := logging.L("gateway").With(
		logging.String("tenant_id", s.cfg.TenantID),
		logging.String("conversation_id", req.Conversation.ConversationID),
	)

	ts, apiErr := validateRequest(req, start)
	if apiErr != nil {
		return model.IngestResponse{}, 0, apiErr
	}

	sessionKey, err := session.ComputeKey(s.cfg.TenantID, req.Conversation, "default")
	if err != nil {
		return model.IngestResponse{}, 0, &APIError{
			StatusCode: http.StatusBadRequest,
			Code:       model.ErrorCodeInvalidArgument,
			Message:    err.Error(),
		}
	}
	if !s.tenantLimiter.Allow(s.cfg.TenantID, start) || !s.sessionLimiter.Allow(sessionKey, start) {
		return model.IngestResponse{}, 0, &APIError{
			StatusCode: http.StatusTooManyRequests,
			Code:       model.ErrorCodeRateLimited,
			Message:    "rate limited",
			RetryAfter: retryAfterSeconds,
		}
	}

	payloadHash, err := canonicalPayloadHash(req)
	if err != nil {
		return model.IngestResponse{}, 0, &APIError{
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
			return model.IngestResponse{}, 0, &APIError{
				StatusCode: http.StatusConflict,
				Code:       model.ErrorCodeConflict,
				Message:    msgIdempotencyConflict,
			}
		}
		return model.IngestResponse{}, 0, &APIError{
			StatusCode: http.StatusInternalServerError,
			Code:       model.ErrorCodeInternal,
			Message:    err.Error(),
		}
	}
	if result.Duplicate {
		return model.IngestResponse{
			EventID:         result.EventID,
			SessionKey:      result.SessionKey,
			ActiveSessionID: result.SessionID,
			ReceivedAt:      result.ReceivedAt.Format(time.RFC3339Nano),
			PayloadHash:     result.PayloadHash,
			Status:          ingestStatusDuplicate,
			StatusURL:       fmt.Sprintf(ingestStatusURLTemplate, result.EventID),
		}, http.StatusOK, nil
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
	return model.IngestResponse{
		EventID:         result.EventID,
		SessionKey:      result.SessionKey,
		ActiveSessionID: result.SessionID,
		ReceivedAt:      result.ReceivedAt.Format(time.RFC3339Nano),
		PayloadHash:     result.PayloadHash,
		Status:          ingestStatusAccepted,
		StatusURL:       fmt.Sprintf(ingestStatusURLTemplate, result.EventID),
	}, http.StatusAccepted, nil
}
