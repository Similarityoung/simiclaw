package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/bus"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/idempotency"
	"github.com/similarityyoung/simiclaw/pkg/model"
	"github.com/similarityyoung/simiclaw/pkg/routing"
	"github.com/similarityyoung/simiclaw/pkg/runtime"
	"github.com/similarityyoung/simiclaw/pkg/store"
)

var (
	tgKeyRE  = regexp.MustCompile(`^telegram:update:[0-9]+$`)
	cliKeyRE = regexp.MustCompile(`^cli:[^:]+:[0-9]+$`)
)

type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Details    map[string]any
	RetryAfter int
}

func (e *APIError) Error() string {
	return e.Message
}

type Service struct {
	cfg           config.Config
	eventBus      *bus.MessageBus
	idempotency   *idempotency.Store
	sessions      *store.SessionStore
	events        *runtime.EventRepo
	tenantLimiter *limiter
	sessLimiter   *limiter
}

func NewService(cfg config.Config, eventBus *bus.MessageBus, idem *idempotency.Store, sessions *store.SessionStore, events *runtime.EventRepo) *Service {
	return &Service{
		cfg:           cfg,
		eventBus:      eventBus,
		idempotency:   idem,
		sessions:      sessions,
		events:        events,
		tenantLimiter: newLimiter(cfg.RateLimitTenantRPS, cfg.RateLimitTenantBurst),
		sessLimiter:   newLimiter(cfg.RateLimitSessionRPS, cfg.RateLimitSessionBurst),
	}
}

func (s *Service) Ingest(ctx context.Context, req model.IngestRequest) (model.IngestResponse, int, *APIError) {
	now := time.Now().UTC()
	ts, apiErr := validateRequest(req, now)
	if apiErr != nil {
		return model.IngestResponse{}, 0, apiErr
	}

	sessionKey, err := routing.ComputeKey(s.cfg.TenantID, req.Conversation, "default")
	if err != nil {
		return model.IngestResponse{}, 0, &APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: err.Error()}
	}

	if !s.tenantLimiter.Allow(s.cfg.TenantID, now) || !s.sessLimiter.Allow(sessionKey, now) {
		return model.IngestResponse{}, 0, &APIError{StatusCode: 429, Code: model.ErrorCodeRateLimited, Message: "rate limited", RetryAfter: 1}
	}

	payloadHash, err := canonicalPayloadHash(req)
	if err != nil {
		return model.IngestResponse{}, 0, &APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: "invalid payload"}
	}

	if existing, ok := s.idempotency.LookupInbound(req.IdempotencyKey); ok {
		if existing.PayloadHash != payloadHash {
			return model.IngestResponse{}, 0, &APIError{
				StatusCode: 409,
				Code:       model.ErrorCodeConflict,
				Message:    "idempotency payload hash mismatch",
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
			Status:          "duplicate_acked",
			StatusURL:       fmt.Sprintf("/v1/events/%s", existing.EventID),
		}
		return resp, 200, nil
	}

	eventID := fmt.Sprintf("evt_%d", now.UnixNano())
	sessionID, _, err := s.sessions.ResolveSession(sessionKey, now)
	if err != nil {
		return model.IngestResponse{}, 0, &APIError{StatusCode: 500, Code: model.ErrorCodeInternal, Message: err.Error()}
	}

	row, dup, err := s.idempotency.RegisterInbound(req.IdempotencyKey, payloadHash, eventID, sessionKey, sessionID, now)
	if err != nil {
		if errors.Is(err, idempotency.ErrConflict) {
			return model.IngestResponse{}, 0, &APIError{StatusCode: 409, Code: model.ErrorCodeConflict, Message: "idempotency payload hash mismatch"}
		}
		return model.IngestResponse{}, 0, &APIError{StatusCode: 500, Code: model.ErrorCodeInternal, Message: err.Error()}
	}
	if dup {
		return model.IngestResponse{
			EventID:         row.EventID,
			SessionKey:      row.SessionKey,
			ActiveSessionID: row.ActiveSessionID,
			ReceivedAt:      row.ReceivedAt.Format(time.RFC3339Nano),
			PayloadHash:     row.PayloadHash,
			Status:          "duplicate_acked",
			StatusURL:       fmt.Sprintf("/v1/events/%s", row.EventID),
		}, 200, nil
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
			return model.IngestResponse{}, 0, &APIError{StatusCode: 500, Code: model.ErrorCodeInternal, Message: fmt.Sprintf("persist event failed: %v; rollback idempotency failed: %v", err, rollbackErr)}
		}
		return model.IngestResponse{}, 0, &APIError{StatusCode: 500, Code: model.ErrorCodeInternal, Message: err.Error()}
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
			Status:          "accepted",
			StatusURL:       fmt.Sprintf("/v1/events/%s", eventID),
		}
		return resp, 202, nil
	}

	publishAPIErr := &APIError{StatusCode: 500, Code: model.ErrorCodeInternal, Message: publishErr.Error()}
	errorCode := model.ErrorCodeInternal
	if errors.Is(publishErr, context.DeadlineExceeded) {
		publishAPIErr = &APIError{StatusCode: 503, Code: model.ErrorCodeQueueFull, Message: "event queue full", RetryAfter: 1}
		errorCode = model.ErrorCodeQueueFull
	} else if errors.Is(publishErr, bus.ErrBusClosed) {
		publishAPIErr = &APIError{StatusCode: 503, Code: model.ErrorCodeQueueUnavailable, Message: "event queue unavailable", RetryAfter: 1}
		errorCode = model.ErrorCodeQueueUnavailable
	} else if errors.Is(publishErr, context.Canceled) && ctx.Err() != nil {
		publishAPIErr = &APIError{StatusCode: 499, Code: model.ErrorCodeCanceled, Message: "request canceled"}
		errorCode = model.ErrorCodeCanceled
	}
	_ = s.events.Update(eventID, func(r *model.EventRecord) {
		r.Status = model.EventStatusFailed
		r.Error = &model.ErrorBlock{Code: errorCode, Message: publishErr.Error()}
	})
	if rollbackErr := s.idempotency.DeleteInbound(req.IdempotencyKey); rollbackErr != nil {
		return model.IngestResponse{}, 0, &APIError{
			StatusCode: 500,
			Code:       model.ErrorCodeInternal,
			Message:    fmt.Sprintf("publish failed: %v; rollback idempotency failed: %v", publishErr, rollbackErr),
		}
	}
	return model.IngestResponse{}, 0, publishAPIErr
}

func validateRequest(req model.IngestRequest, now time.Time) (time.Time, *APIError) {
	if req.Source == "" {
		return time.Time{}, &APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: "field source is required", Details: map[string]any{"field": "source"}}
	}
	if req.Conversation.ConversationID == "" {
		return time.Time{}, &APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: "field conversation.conversation_id is required", Details: map[string]any{"field": "conversation.conversation_id"}}
	}
	if req.Conversation.ChannelType == "" {
		return time.Time{}, &APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: "field conversation.channel_type is required", Details: map[string]any{"field": "conversation.channel_type"}}
	}
	if req.Conversation.ChannelType == "channel" {
		return time.Time{}, &APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: "channel_type 'channel' is reserved", Details: map[string]any{"field": "conversation.channel_type"}}
	}
	if req.Conversation.ChannelType == "dm" && req.Conversation.ParticipantID == "" {
		return time.Time{}, &APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: "field conversation.participant_id is required", Details: map[string]any{"field": "conversation.participant_id"}}
	}
	if req.Payload.Type == "" {
		return time.Time{}, &APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: "field payload.type is required", Details: map[string]any{"field": "payload.type"}}
	}
	if req.IdempotencyKey == "" {
		return time.Time{}, &APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: "field idempotency_key is required", Details: map[string]any{"field": "idempotency_key"}}
	}
	if req.Source == "telegram" && !tgKeyRE.MatchString(req.IdempotencyKey) {
		return time.Time{}, &APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: "invalid telegram idempotency_key format"}
	}
	if req.Source == "cli" && !cliKeyRE.MatchString(req.IdempotencyKey) {
		return time.Time{}, &APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: "invalid cli idempotency_key format"}
	}
	if req.Payload.NativeRef != "" {
		clean := filepath.Clean(req.Payload.NativeRef)
		if strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) || !strings.HasPrefix(clean, "runtime/native/") {
			return time.Time{}, &APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: "native_ref must stay within runtime/native/**"}
		}
	}
	if req.Timestamp == "" {
		return time.Time{}, &APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: "field timestamp is required", Details: map[string]any{"field": "timestamp"}}
	}
	if !strings.HasSuffix(req.Timestamp, "Z") {
		return time.Time{}, &APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: "timestamp must be UTC"}
	}
	ts, err := time.Parse(time.RFC3339, req.Timestamp)
	if err != nil {
		return time.Time{}, &APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: "invalid timestamp"}
	}
	if diff := now.Sub(ts); diff > 10*time.Minute || diff < -10*time.Minute {
		return time.Time{}, &APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: "timestamp drift exceeds 10m"}
	}
	return ts, nil
}

func canonicalPayloadHash(req model.IngestRequest) (string, error) {
	shape := struct {
		Source         string             `json:"source"`
		Conversation   model.Conversation `json:"conversation"`
		Payload        model.EventPayload `json:"payload"`
		IdempotencyKey string             `json:"idempotency_key"`
	}{
		Source:         req.Source,
		Conversation:   req.Conversation,
		Payload:        req.Payload,
		IdempotencyKey: req.IdempotencyKey,
	}
	b, err := json.Marshal(shape)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
