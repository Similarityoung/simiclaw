package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/similarityyoung/simiclaw/internal/session"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const (
	retryAfterSeconds       = 1
	maxTimestampDriftWindow = 10 * time.Minute
)

var (
	tgKeyRE             = regexp.MustCompile(`^telegram:update:[0-9]+$`)
	cliKeyRE            = regexp.MustCompile(`^cli:[^:]+:[0-9]+$`)
	windowsVolumePathRE = regexp.MustCompile(`^[A-Za-z]:/`)
)

type Command struct {
	Request    api.IngestRequest
	ReceivedAt time.Time
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

type Error struct {
	Code       string
	Message    string
	Details    map[string]any
	RetryAfter int
}

func (e *Error) Error() string {
	return e.Message
}

type Enqueuer interface {
	TryEnqueue(eventID string) bool
}

type ScopeResolver interface {
	Resolve(ctx context.Context, req api.IngestRequest) (api.IngestRequest, string, *Error)
}

type Service struct {
	tenantID       string
	repo           Repository
	queue          Enqueuer
	scopeResolver  ScopeResolver
	tenantLimiter  *limiter
	sessionLimiter *limiter
	now            func() time.Time
}

func NewService(
	tenantID string,
	repo Repository,
	queue Enqueuer,
	scopeResolver ScopeResolver,
	tenantRate float64,
	tenantBurst float64,
	sessionRate float64,
	sessionBurst float64,
) *Service {
	return &Service{
		tenantID:       tenantID,
		repo:           repo,
		queue:          queue,
		scopeResolver:  scopeResolver,
		tenantLimiter:  newLimiter(tenantRate, tenantBurst),
		sessionLimiter: newLimiter(sessionRate, sessionBurst),
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *Service) Ingest(ctx context.Context, cmd Command) (Result, *Error) {
	now := cmd.ReceivedAt.UTC()
	if now.IsZero() {
		now = s.now()
	}
	if cmd.Request.Payload.NativeRef != "" {
		cmd.Request.Payload.NativeRef = normalizeNativeRef(cmd.Request.Payload.NativeRef)
	}

	ts, err := validateRequest(cmd.Request, now)
	if err != nil {
		return Result{}, err
	}

	req, scope, err := s.scopeResolver.Resolve(ctx, cmd.Request)
	if err != nil {
		return Result{}, err
	}

	sessionKey, computeErr := session.ComputeKey(s.tenantID, req.Conversation, scope)
	if computeErr != nil {
		return Result{}, &Error{
			Code:    model.ErrorCodeInvalidArgument,
			Message: computeErr.Error(),
		}
	}

	sessionLimitKey, computeErr := sessionRateLimitKey(s.tenantID, req)
	if computeErr != nil {
		return Result{}, &Error{
			Code:    model.ErrorCodeInvalidArgument,
			Message: computeErr.Error(),
		}
	}
	if !s.tenantLimiter.Allow(s.tenantID, now) || !s.sessionLimiter.Allow(sessionLimitKey, now) {
		return Result{}, &Error{
			Code:       model.ErrorCodeRateLimited,
			Message:    "rate limited",
			RetryAfter: retryAfterSeconds,
		}
	}
	persistReq := PersistRequest{
		Source:         req.Source,
		Conversation:   req.Conversation,
		Payload:        req.Payload,
		IdempotencyKey: req.IdempotencyKey,
		DMScope:        req.DMScope,
	}

	payloadHash, hashErr := canonicalPayloadHash(req)
	if hashErr != nil {
		return Result{}, &Error{
			Code:    model.ErrorCodeInvalidArgument,
			Message: "invalid payload",
		}
	}

	ingestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	stored, ingestErr := s.repo.PersistEvent(ingestCtx, s.tenantID, sessionKey, persistReq, payloadHash, ts)
	if ingestErr != nil {
		if errors.Is(ingestErr, ErrIdempotencyConflict) {
			return Result{}, &Error{
				Code:    model.ErrorCodeConflict,
				Message: "idempotency payload hash mismatch",
			}
		}
		return Result{}, &Error{
			Code:    model.ErrorCodeInternal,
			Message: ingestErr.Error(),
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
	if stored.Duplicate {
		return result, nil
	}

	if s.queue != nil && s.queue.TryEnqueue(stored.EventID) {
		result.Enqueued = true
		_ = s.repo.MarkEventQueued(ctx, stored.EventID, now)
	}
	return result, nil
}

func validateRequest(req api.IngestRequest, now time.Time) (time.Time, *Error) {
	if req.Source == "" {
		return time.Time{}, invalidArgument("field source is required", "source")
	}
	if req.Conversation.ConversationID == "" {
		return time.Time{}, invalidArgument("field conversation.conversation_id is required", "conversation.conversation_id")
	}
	if req.Conversation.ChannelType == "" {
		return time.Time{}, invalidArgument("field conversation.channel_type is required", "conversation.channel_type")
	}
	if req.Conversation.ChannelType == "channel" {
		return time.Time{}, invalidArgument("channel_type 'channel' is reserved", "conversation.channel_type")
	}
	if req.Conversation.ChannelType == "dm" && req.Conversation.ParticipantID == "" {
		return time.Time{}, invalidArgument("field conversation.participant_id is required", "conversation.participant_id")
	}
	if req.Payload.Type == "" {
		return time.Time{}, invalidArgument("field payload.type is required", "payload.type")
	}
	if req.IdempotencyKey == "" {
		return time.Time{}, invalidArgument("field idempotency_key is required", "idempotency_key")
	}
	if req.Source == "telegram" && !tgKeyRE.MatchString(req.IdempotencyKey) {
		return time.Time{}, &Error{Code: model.ErrorCodeInvalidArgument, Message: "invalid telegram idempotency_key format"}
	}
	if req.Source == "cli" && !cliKeyRE.MatchString(req.IdempotencyKey) {
		return time.Time{}, &Error{Code: model.ErrorCodeInvalidArgument, Message: "invalid cli idempotency_key format"}
	}
	if req.Payload.NativeRef != "" {
		clean := normalizeNativeRef(req.Payload.NativeRef)
		if clean == ".." ||
			strings.HasPrefix(clean, "../") ||
			strings.HasPrefix(clean, "/") ||
			windowsVolumePathRE.MatchString(clean) ||
			!strings.HasPrefix(clean, "runtime/native/") {
			return time.Time{}, &Error{Code: model.ErrorCodeInvalidArgument, Message: "native_ref must stay within runtime/native/**"}
		}
	}
	if req.Timestamp == "" {
		return time.Time{}, invalidArgument("field timestamp is required", "timestamp")
	}
	if !strings.HasSuffix(req.Timestamp, "Z") {
		return time.Time{}, &Error{Code: model.ErrorCodeInvalidArgument, Message: "timestamp must be UTC"}
	}
	ts, err := time.Parse(time.RFC3339Nano, req.Timestamp)
	if err != nil {
		return time.Time{}, &Error{Code: model.ErrorCodeInvalidArgument, Message: "invalid timestamp"}
	}
	if diff := now.Sub(ts); diff > maxTimestampDriftWindow || diff < -maxTimestampDriftWindow {
		return time.Time{}, &Error{Code: model.ErrorCodeInvalidArgument, Message: "timestamp drift exceeds 10m"}
	}
	return ts, nil
}

func normalizeNativeRef(ref string) string {
	return path.Clean(strings.ReplaceAll(ref, "\\", "/"))
}

func canonicalPayloadHash(req api.IngestRequest) (string, error) {
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

func sessionRateLimitKey(tenantID string, req api.IngestRequest) (string, error) {
	return session.ComputeKey(tenantID, req.Conversation, session.DefaultScope)
}

func invalidArgument(message, field string) *Error {
	return &Error{
		Code:    model.ErrorCodeInvalidArgument,
		Message: message,
		Details: map[string]any{"field": field},
	}
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

const gcInterval = 256
const bucketExpiry = 5 * time.Minute

func newLimiter(rate, burst float64) *limiter {
	return &limiter{rate: rate, burst: burst, buckets: map[string]*bucket{}}
}

func (l *limiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.gcCounter++
	if l.gcCounter >= gcInterval {
		l.gcCounter = 0
		for k, b := range l.buckets {
			if now.Sub(b.last) > bucketExpiry {
				delete(l.buckets, k)
			}
		}
	}
	b, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &bucket{tokens: l.burst - 1, last: now}
		return true
	}
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens = math.Min(l.burst, b.tokens+elapsed*l.rate)
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
