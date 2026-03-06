package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/idempotency"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
	store "github.com/similarityyoung/simiclaw/pkg/persistence"
	"github.com/similarityyoung/simiclaw/pkg/sessionkey"
)

func (s *Service) Ingest(ctx context.Context, req model.IngestRequest) (model.IngestResponse, int, *APIError) {
	start := time.Now()
	now := start.UTC()
	logger := logging.L("gateway").With(
		logging.String("tenant_id", s.cfg.TenantID),
		logging.String("conversation_id", req.Conversation.ConversationID),
	)

	_, apiErr := validateRequest(req, now)
	if apiErr != nil {
		logger.Warn("gateway.ingest.failed",
			logging.String("status", "failed"),
			logging.String("error_code", apiErr.Code),
			logging.Int("http_status", apiErr.StatusCode),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
		return model.IngestResponse{}, 0, apiErr
	}

	sessionKey, err := sessionkey.ComputeSessionKey(s.cfg.TenantID, req.Conversation, "default")
	if err != nil {
		logger.Warn("gateway.ingest.failed",
			logging.String("status", "failed"),
			logging.String("error_code", model.ErrorCodeInvalidArgument),
			logging.Error(err),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
		return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: err.Error()}
	}
	logger = logger.With(logging.String("session_key", sessionKey))

	if !s.tenantLimiter.Allow(s.cfg.TenantID, now) || !s.sessionLimiter.Allow(sessionKey, now) {
		logger.Warn("gateway.ingest.rate_limited",
			logging.String("status", "rate_limited"),
			logging.String("error_code", model.ErrorCodeRateLimited),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
		return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusTooManyRequests, Code: model.ErrorCodeRateLimited, Message: "rate limited", RetryAfter: retryAfterSeconds}
	}

	payloadHash, err := canonicalPayloadHash(req)
	if err != nil {
		logger.Warn("gateway.ingest.failed",
			logging.String("status", "failed"),
			logging.String("error_code", model.ErrorCodeInvalidArgument),
			logging.Error(err),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
		return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "invalid payload"}
	}

	if existing, ok := s.idempotency.LookupInbound(req.IdempotencyKey); ok {
		if existing.PayloadHash != payloadHash {
			logger.Warn("gateway.ingest.failed",
				logging.String("status", "failed"),
				logging.String("error_code", model.ErrorCodeConflict),
				logging.String("event_id", existing.EventID),
				logging.String("session_id", existing.ActiveSessionID),
				logging.Int64("latency_ms", time.Since(start).Milliseconds()),
			)
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
		logger.Info("gateway.ingest.duplicate",
			logging.String("status", ingestStatusDuplicate),
			logging.String("event_id", existing.EventID),
			logging.String("session_id", existing.ActiveSessionID),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
		return resp, http.StatusOK, nil
	}

	eventID := fmt.Sprintf("evt_%d", now.UnixNano())
	sessionID, _, err := s.sessions.ResolveSession(sessionKey, req.Conversation, "default", now)
	if err != nil {
		logger.Error("gateway.ingest.failed",
			logging.String("status", "failed"),
			logging.String("event_id", eventID),
			logging.String("error_code", model.ErrorCodeInternal),
			logging.Error(err),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
		return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: err.Error()}
	}
	logger = logger.With(
		logging.String("event_id", eventID),
		logging.String("session_id", sessionID),
	)
	if s.adkRouter == nil {
		logger.Error("gateway.ingest.failed",
			logging.String("status", "failed"),
			logging.String("error_code", model.ErrorCodeInternal),
			logging.String("reason", "adk router not configured"),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
		_ = s.idempotency.DeleteInbound(req.IdempotencyKey)
		return model.IngestResponse{}, 0, &APIError{
			StatusCode: http.StatusInternalServerError,
			Code:       model.ErrorCodeInternal,
			Message:    "adk router is not configured",
		}
	}
	if err := s.adkRouter.RouteIngest(ctx, req, sessionKey, sessionID); err != nil {
		logger.Error("gateway.ingest.failed",
			logging.String("status", "failed"),
			logging.String("error_code", model.ErrorCodeInternal),
			logging.Error(err),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
		_ = s.idempotency.DeleteInbound(req.IdempotencyKey)
		return model.IngestResponse{}, 0, &APIError{
			StatusCode: http.StatusInternalServerError,
			Code:       model.ErrorCodeInternal,
			Message:    fmt.Sprintf("adk gateway route failed: %v", err),
		}
	}

	row, dup, err := s.idempotency.RegisterInbound(req.IdempotencyKey, payloadHash, eventID, sessionKey, sessionID, now)
	if err != nil {
		if errors.Is(err, idempotency.ErrConflict) {
			logger.Warn("gateway.ingest.failed",
				logging.String("status", "failed"),
				logging.String("error_code", model.ErrorCodeConflict),
				logging.Int64("latency_ms", time.Since(start).Milliseconds()),
			)
			return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusConflict, Code: model.ErrorCodeConflict, Message: msgIdempotencyConflict}
		}
		logger.Error("gateway.ingest.failed",
			logging.String("status", "failed"),
			logging.String("error_code", model.ErrorCodeInternal),
			logging.Error(err),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
		return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: err.Error()}
	}
	if dup {
		logger.Info("gateway.ingest.duplicate",
			logging.String("status", ingestStatusDuplicate),
			logging.String("event_id", row.EventID),
			logging.String("session_id", row.ActiveSessionID),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
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
			logger.Error("gateway.ingest.failed",
				logging.String("status", "failed"),
				logging.String("error_code", model.ErrorCodeInternal),
				logging.Error(err),
				logging.NamedError("rollback_error", rollbackErr),
				logging.Int64("latency_ms", time.Since(start).Milliseconds()),
			)
			return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: fmt.Sprintf("persist event failed: %v; rollback idempotency failed: %v", err, rollbackErr)}
		}
		logger.Error("gateway.ingest.failed",
			logging.String("status", "failed"),
			logging.String("error_code", model.ErrorCodeInternal),
			logging.Error(err),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
		return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: err.Error()}
	}

	runResult, err := s.adkRouter.RunIngest(ctx, req, sessionKey, sessionID)
	if err != nil {
		_ = s.events.Update(eventID, func(r *model.EventRecord) {
			r.Status = model.EventStatusFailed
			r.UpdatedAt = time.Now().UTC()
			r.Error = &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: err.Error()}
		})
		_ = s.idempotency.DeleteInbound(req.IdempotencyKey)
		logger.Error("gateway.ingest.failed",
			logging.String("status", "failed"),
			logging.String("error_code", model.ErrorCodeInternal),
			logging.Error(err),
			logging.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
		return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: fmt.Sprintf("adk execution failed: %v", err)}
	}
	if s.storeLoop == nil {
		_ = s.idempotency.DeleteInbound(req.IdempotencyKey)
		return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: "store loop is not configured"}
	}
	commitID, err := s.storeLoop.Commit(ctx, store.CommitRequest{
		SessionKey: sessionKey,
		SessionID:  sessionID,
		Entries:    runResult.Entries,
		RunTrace:   runResult.Trace,
		Now:        time.Now().UTC(),
	})
	if err != nil {
		_ = s.events.Update(eventID, func(r *model.EventRecord) {
			r.Status = model.EventStatusFailed
			r.UpdatedAt = time.Now().UTC()
			r.Error = &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: err.Error()}
		})
		_ = s.idempotency.DeleteInbound(req.IdempotencyKey)
		return model.IngestResponse{}, 0, &APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: err.Error()}
	}
	_ = s.events.Update(eventID, func(r *model.EventRecord) {
		r.Status = model.EventStatusCommitted
		r.RunID = runResult.RunID
		r.RunMode = runResult.RunMode
		r.CommitID = commitID
		r.UpdatedAt = time.Now().UTC()
		r.AssistantReply = runResult.OutboundBody
		r.Error = nil
		if runResult.SuppressOutput {
			r.DeliveryStatus = model.DeliveryStatusSuppressed
			r.DeliveryDetail = model.DeliveryDetailNotApplicable
		} else {
			r.DeliveryStatus = model.DeliveryStatusSent
			r.DeliveryDetail = model.DeliveryDetailDirect
		}
	})
	resp := model.IngestResponse{
		EventID:         eventID,
		SessionKey:      sessionKey,
		ActiveSessionID: sessionID,
		ReceivedAt:      now.Format(time.RFC3339Nano),
		PayloadHash:     payloadHash,
		Status:          ingestStatusAccepted,
		StatusURL:       fmt.Sprintf(ingestStatusURLTemplate, eventID),
	}
	logger.Info("gateway.ingest.accepted",
		logging.String("status", ingestStatusAccepted),
		logging.Int64("latency_ms", time.Since(start).Milliseconds()),
	)
	return resp, http.StatusAccepted, nil
}
