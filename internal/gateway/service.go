package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/similarityyoung/simiclaw/internal/gateway/bindings"
	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
	"github.com/similarityyoung/simiclaw/internal/gateway/routing"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const (
	retryAfterSeconds       = 1
	maxTimestampDriftWindow = 10 * time.Minute
	gcInterval              = 256
	bucketExpiry            = 5 * time.Minute
)

var (
	tgKeyRE             = regexp.MustCompile(`^telegram:update:[0-9]+$`)
	cliKeyRE            = regexp.MustCompile(`^cli:[^:]+:[0-9]+$`)
	windowsVolumePathRE = regexp.MustCompile(`^[A-Za-z]:/`)
)

type Enqueuer interface {
	TryEnqueue(eventID string) bool
}

type Result struct {
	EventID     string
	SessionKey  string
	SessionID   string
	ReceivedAt  time.Time
	PayloadHash string
	Duplicate   bool
	Enqueued    bool
}

type AcceptedIngest struct {
	Response   api.IngestResponse
	Result     Result
	StatusCode int
}

type Service struct {
	tenantID       string
	repo           Repository
	queue          Enqueuer
	bindings       bindings.Resolver
	routes         routing.Resolver
	tenantLimiter  *limiter
	sessionLimiter *limiter
	now            func() time.Time
}

type limiter struct {
	mu        sync.Mutex
	rate      float64
	burst     float64
	buckets   map[string]*bucket
	gcCounter int
}

type bucket struct {
	tokens float64
	last   time.Time
}

func NewService(
	tenantID string,
	repo Repository,
	queue Enqueuer,
	bindingsResolver bindings.Resolver,
	routeResolver routing.Resolver,
	tenantRate float64,
	tenantBurst float64,
	sessionRate float64,
	sessionBurst float64,
) *Service {
	return &Service{
		tenantID:       tenantID,
		repo:           repo,
		queue:          queue,
		bindings:       bindingsResolver,
		routes:         routeResolver,
		tenantLimiter:  newLimiter(tenantRate, tenantBurst),
		sessionLimiter: newLimiter(sessionRate, sessionBurst),
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *Service) SetClock(now func() time.Time) {
	if now == nil {
		return
	}
	s.now = now
}

func (s *Service) Accept(ctx context.Context, in gatewaymodel.NormalizedIngress) (AcceptedIngest, *APIError) {
	if s.repo == nil {
		return AcceptedIngest{}, &APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: "gateway repository unavailable"}
	}
	now := s.now().UTC()
	in.Payload.NativeRef = normalizeNativeRef(in.Payload.NativeRef)

	ts, apiErr := validateIngress(in, now)
	if apiErr != nil {
		return AcceptedIngest{}, apiErr
	}
	binding, err := s.bindings.Resolve(ctx, in)
	if err != nil {
		return AcceptedIngest{}, &APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: err.Error()}
	}
	decision, err := s.routes.Resolve(ctx, in, binding)
	if err != nil {
		return AcceptedIngest{}, &APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: err.Error()}
	}

	sessionLimitKey, err := sessionRateLimitKey(s.tenantID, in.Conversation)
	if err != nil {
		return AcceptedIngest{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: err.Error()}
	}
	if !s.tenantLimiter.Allow(s.tenantID, now) || !s.sessionLimiter.Allow(sessionLimitKey, now) {
		return AcceptedIngest{}, &APIError{
			StatusCode: http.StatusTooManyRequests,
			Code:       model.ErrorCodeRateLimited,
			Message:    "rate limited",
			RetryAfter: retryAfterSeconds,
		}
	}

	effective := in
	if decision.PayloadType != "" {
		effective.Payload.Type = decision.PayloadType
	}
	payloadHash, err := canonicalPayloadHash(effective)
	if err != nil {
		return AcceptedIngest{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "invalid payload"}
	}

	persistReq := PersistRequest{
		Source:         in.Source,
		Conversation:   in.Conversation,
		Payload:        effective.Payload,
		IdempotencyKey: in.IdempotencyKey,
		DMScope:        binding.Scope,
	}
	ingestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	stored, err := s.repo.PersistEvent(ingestCtx, binding.TenantID, binding.SessionKey, persistReq, payloadHash, ts)
	if err != nil {
		if errors.Is(err, ErrIdempotencyConflict) {
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

	result := Result{
		EventID:     stored.EventID,
		SessionKey:  stored.SessionKey,
		SessionID:   stored.SessionID,
		ReceivedAt:  stored.ReceivedAt,
		PayloadHash: stored.PayloadHash,
		Duplicate:   stored.Duplicate,
	}
	if !stored.Duplicate && s.queue != nil && s.queue.TryEnqueue(stored.EventID) {
		result.Enqueued = true
		_ = s.repo.MarkEventQueued(ctx, stored.EventID, now)
	}

	status := ingestStatusAccepted
	statusCode := http.StatusAccepted
	if result.Duplicate {
		status = ingestStatusDuplicate
		statusCode = http.StatusOK
	}
	return AcceptedIngest{
		Response: api.IngestResponse{
			EventID:         result.EventID,
			SessionKey:      result.SessionKey,
			ActiveSessionID: result.SessionID,
			ReceivedAt:      result.ReceivedAt.Format(time.RFC3339Nano),
			PayloadHash:     result.PayloadHash,
			Status:          status,
			StatusURL:       statusURLFor(result.EventID),
		},
		Result:     result,
		StatusCode: statusCode,
	}, nil
}

func statusURLFor(eventID string) string {
	return "/v1/events/" + eventID
}

func validateIngress(in gatewaymodel.NormalizedIngress, now time.Time) (time.Time, *APIError) {
	if in.Source == "" {
		return time.Time{}, invalidArgument("field source is required", "source")
	}
	if in.Conversation.ConversationID == "" {
		return time.Time{}, invalidArgument("field conversation.conversation_id is required", "conversation.conversation_id")
	}
	if in.Conversation.ChannelType == "" {
		return time.Time{}, invalidArgument("field conversation.channel_type is required", "conversation.channel_type")
	}
	if in.Conversation.ChannelType == "channel" {
		return time.Time{}, invalidArgument("channel_type 'channel' is reserved", "conversation.channel_type")
	}
	if in.Conversation.ChannelType == "dm" && in.Conversation.ParticipantID == "" {
		return time.Time{}, invalidArgument("field conversation.participant_id is required", "conversation.participant_id")
	}
	if in.Payload.Type == "" {
		return time.Time{}, invalidArgument("field payload.type is required", "payload.type")
	}
	if in.IdempotencyKey == "" {
		return time.Time{}, invalidArgument("field idempotency_key is required", "idempotency_key")
	}
	if in.Source == "telegram" && !tgKeyRE.MatchString(in.IdempotencyKey) {
		return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "invalid telegram idempotency_key format"}
	}
	if in.Source == "cli" && !cliKeyRE.MatchString(in.IdempotencyKey) {
		return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "invalid cli idempotency_key format"}
	}
	if in.Payload.NativeRef != "" {
		clean := normalizeNativeRef(in.Payload.NativeRef)
		if clean == ".." ||
			strings.HasPrefix(clean, "../") ||
			strings.HasPrefix(clean, "/") ||
			windowsVolumePathRE.MatchString(clean) ||
			!strings.HasPrefix(clean, "runtime/native/") {
			return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "native_ref must stay within runtime/native/**"}
		}
	}
	if in.Timestamp.IsZero() {
		return time.Time{}, invalidArgument("field timestamp is required", "timestamp")
	}
	ts := in.Timestamp.UTC()
	if diff := now.Sub(ts); diff > maxTimestampDriftWindow || diff < -maxTimestampDriftWindow {
		return time.Time{}, &APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "timestamp drift exceeds 10m"}
	}
	return ts, nil
}

func normalizeNativeRef(ref string) string {
	if strings.TrimSpace(ref) == "" {
		return ""
	}
	return path.Clean(strings.ReplaceAll(ref, "\\", "/"))
}

func canonicalPayloadHash(in gatewaymodel.NormalizedIngress) (string, error) {
	shape := struct {
		Source         string             `json:"source"`
		Conversation   model.Conversation `json:"conversation"`
		Payload        model.EventPayload `json:"payload"`
		IdempotencyKey string             `json:"idempotency_key"`
	}{
		Source:         in.Source,
		Conversation:   in.Conversation,
		Payload:        in.Payload,
		IdempotencyKey: in.IdempotencyKey,
	}
	body, err := json.Marshal(shape)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func sessionRateLimitKey(tenantID string, conv model.Conversation) (string, error) {
	return bindings.ComputeKey(tenantID, conv, bindings.DefaultScope)
}

func invalidArgument(message, field string) *APIError {
	return &APIError{
		StatusCode: http.StatusBadRequest,
		Code:       model.ErrorCodeInvalidArgument,
		Message:    message,
		Details:    map[string]any{"field": field},
	}
}

func newLimiter(rate, burst float64) *limiter {
	return &limiter{rate: rate, burst: burst, buckets: map[string]*bucket{}}
}

func (l *limiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.gcCounter++
	if l.gcCounter >= gcInterval {
		l.gcCounter = 0
		for id, entry := range l.buckets {
			if now.Sub(entry.last) > bucketExpiry {
				delete(l.buckets, id)
			}
		}
	}
	entry, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &bucket{tokens: l.burst - 1, last: now}
		return true
	}
	elapsed := now.Sub(entry.last).Seconds()
	if elapsed > 0 {
		entry.tokens = math.Min(l.burst, entry.tokens+elapsed*l.rate)
		entry.last = now
	}
	if entry.tokens < 1 {
		return false
	}
	entry.tokens--
	return true
}
