package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/bus"
	"github.com/similarityyoung/simiclaw/pkg/idempotency"
	"github.com/similarityyoung/simiclaw/pkg/model"
	"github.com/similarityyoung/simiclaw/pkg/sessionkey"
)

func (s *Service) Ingest(ctx context.Context, req model.IngestRequest) (model.IngestResponse, int, *APIError) {
	now := time.Now().UTC()
	ts, apiErr := validateRequest(req, now)
	if apiErr != nil {
		return model.IngestResponse{}, 0, apiErr
	}

	sessionKey, err := sessionkey.ComputeSessionKey(s.cfg.TenantID, req.Conversation, "default")
	if err != nil {
		return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: err.Error()}
	}

	if !s.tenantLimiter.Allow(s.cfg.TenantID, now) || !s.sessionLimiter.Allow(sessionKey, now) {
		return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusTooManyRequests, Code: model.ErrorCodeRateLimited, Message: "rate limited", RetryAfter: retryAfterSeconds}
	}

	payloadHash, err := canonicalPayloadHash(req)
	if err != nil {
		return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "invalid payload"}
	}

	if existing, ok := s.idempotency.LookupInbound(req.IdempotencyKey); ok {
		if existing.PayloadHash != payloadHash {
			return model.IngestResponse{}, 0, &APIError{
				StatusCode: http.StatusConflict,
				Code:       model.ErrorCodeConflict,
				Message:    msgIdempotencyConflict,
				Details: map[string]any{
					"expected_hash": existing.PayloadHash,
					"got_hash":      payloadHash,
				},
			}
		}
		resp := model.IngestResponse{
			EventID:         existing.EventID,
			SessionKey:      existing.SessionKey,
			ActiveSessionID: existing.ActiveSessionID,
			ReceivedAt:      existing.ReceivedAt.Format(time.RFC3339Nano),
			PayloadHash:     existing.PayloadHash,
			Status:          ingestStatusDuplicate,
			StatusURL:       fmt.Sprintf(ingestStatusURLTemplate, existing.EventID),
		}
		return resp, http.StatusOK, nil
	}

	eventID := fmt.Sprintf("evt_%d", now.UnixNano())
	sessionID, _, err := s.sessions.ResolveSession(sessionKey, now)
	if err != nil {
		return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: err.Error()}
	}

	row, dup, err := s.idempotency.RegisterInbound(req.IdempotencyKey, payloadHash, eventID, sessionKey, sessionID, now)
	if err != nil {
		if errors.Is(err, idempotency.ErrConflict) {
			return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusConflict, Code: model.ErrorCodeConflict, Message: msgIdempotencyConflict}
		}
		return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: err.Error()}
	}
	if dup {
		return model.IngestResponse{
			EventID:         row.EventID,
			SessionKey:      row.SessionKey,
			ActiveSessionID: row.ActiveSessionID,
			ReceivedAt:      row.ReceivedAt.Format(time.RFC3339Nano),
			PayloadHash:     row.PayloadHash,
			Status:          ingestStatusDuplicate,
			StatusURL:       fmt.Sprintf(ingestStatusURLTemplate, row.EventID),
		}, http.StatusOK, nil
	}

	rec := model.EventRecord{
		EventID:        eventID,
		Status:         model.EventStatusAccepted,
		DeliveryStatus: model.DeliveryStatusNotApplicable,
		DeliveryDetail: model.DeliveryDetailNotApplicable,
		SessionKey:     sessionKey,
		SessionID:      sessionID,
		RunMode:        model.RunModeNormal,
		ReceivedAt:     now,
		UpdatedAt:      now,
		PayloadHash:    payloadHash,
	}
	if err := s.events.Put(rec); err != nil {
		if rollbackErr := s.idempotency.DeleteInbound(req.IdempotencyKey); rollbackErr != nil {
			return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: fmt.Sprintf("persist event failed: %v; rollback idempotency failed: %v", err, rollbackErr)}
		}
		return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: err.Error()}
	}

	event := model.InternalEvent{
		EventID:         eventID,
		Source:          req.Source,
		TenantID:        s.cfg.TenantID,
		Conversation:    req.Conversation,
		SessionKey:      sessionKey,
		IdempotencyKey:  req.IdempotencyKey,
		Timestamp:       ts,
		Payload:         req.Payload,
		ActiveSessionID: sessionID,
	}

	enqueueCtx, cancel := context.WithTimeout(ctx, s.cfg.IngestEnqueueTimeout.Duration)
	defer cancel()

	publishErr := s.eventBus.PublishInbound(enqueueCtx, event)
	if publishErr == nil {
		resp := model.IngestResponse{
			EventID:         eventID,
			SessionKey:      sessionKey,
			ActiveSessionID: sessionID,
			ReceivedAt:      now.Format(time.RFC3339Nano),
			PayloadHash:     payloadHash,
			Status:          ingestStatusAccepted,
			StatusURL:       fmt.Sprintf(ingestStatusURLTemplate, eventID),
		}
		return resp, http.StatusAccepted, nil
	}

	publishAPIErr := &APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: publishErr.Error()}
	errorCode := model.ErrorCodeInternal
	if errors.Is(publishErr, context.DeadlineExceeded) {
		publishAPIErr = &APIError{StatusCode: http.StatusServiceUnavailable, Code: model.ErrorCodeQueueFull, Message: "event queue full", RetryAfter: retryAfterSeconds}
		errorCode = model.ErrorCodeQueueFull
	} else if errors.Is(publishErr, bus.ErrBusClosed) {
		publishAPIErr = &APIError{StatusCode: http.StatusServiceUnavailable, Code: model.ErrorCodeQueueUnavailable, Message: "event queue unavailable", RetryAfter: retryAfterSeconds}
		errorCode = model.ErrorCodeQueueUnavailable
	} else if errors.Is(publishErr, context.Canceled) && ctx.Err() != nil {
		publishAPIErr = &APIError{StatusCode: statusClientClosedReq, Code: model.ErrorCodeCanceled, Message: "request canceled"}
		errorCode = model.ErrorCodeCanceled
	}
	_ = s.events.Update(eventID, func(r *model.EventRecord) {
		r.Status = model.EventStatusFailed
		r.Error = &model.ErrorBlock{Code: errorCode, Message: publishErr.Error()}
	})
	if rollbackErr := s.idempotency.DeleteInbound(req.IdempotencyKey); rollbackErr != nil {
		return model.IngestResponse{}, 0, &APIError{
			StatusCode: http.StatusInternalServerError,
			Code:       model.ErrorCodeInternal,
			Message:    fmt.Sprintf("publish failed: %v; rollback idempotency failed: %v", publishErr, rollbackErr),
		}
	}
	return model.IngestResponse{}, 0, publishAPIErr
}
