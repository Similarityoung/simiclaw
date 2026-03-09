package gateway

import (
	"context"
	"net/http"
	"strings"

	sessionpkg "github.com/similarityyoung/simiclaw/internal/session"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const payloadTypeNewSession = "new_session"

func (s *Service) resolveRequestScope(ctx context.Context, req model.IngestRequest) (model.IngestRequest, string, *APIError) {
	if req.Payload.Type == "message" && sessionpkg.IsNewSessionCommand(req.Payload.Text) {
		scope := sessionpkg.NewScopeFromID(req.IdempotencyKey)
		req.DMScope = scope
		req.Payload.Type = payloadTypeNewSession
		return req, scope, nil
	}

	if scope, ok, apiErr := s.scopeFromSessionHint(ctx, req); apiErr != nil {
		return model.IngestRequest{}, "", apiErr
	} else if ok {
		req.DMScope = scope
		return req, scope, nil
	}

	if req.DMScope != "" {
		scope := sessionpkg.ScopeFromRequest(req)
		req.DMScope = scope
		return req, scope, nil
	}

	scope, ok, err := s.db.GetConversationDMScope(ctx, s.cfg.TenantID, req.Conversation)
	if err != nil {
		return model.IngestRequest{}, "", &APIError{
			StatusCode: http.StatusInternalServerError,
			Code:       model.ErrorCodeInternal,
			Message:    err.Error(),
		}
	}
	if !ok {
		scope = sessionpkg.DefaultScope
	}
	req.DMScope = scope
	return req, scope, nil
}

func (s *Service) scopeFromSessionHint(ctx context.Context, req model.IngestRequest) (string, bool, *APIError) {
	key := strings.TrimSpace(req.SessionKeyHint)
	if key == "" {
		return "", false, nil
	}

	rec, ok, err := s.db.GetSession(ctx, key)
	if err != nil {
		return "", false, &APIError{
			StatusCode: http.StatusInternalServerError,
			Code:       model.ErrorCodeInternal,
			Message:    err.Error(),
		}
	}
	if !ok {
		return "", false, nil
	}
	if rec.ConversationID != req.Conversation.ConversationID || rec.ChannelType != req.Conversation.ChannelType || rec.ParticipantID != req.Conversation.ParticipantID {
		return "", false, nil
	}
	return sessionpkg.NormalizeScope(rec.DMScope), true, nil
}

func sessionRateLimitKey(tenantID string, req model.IngestRequest) (string, error) {
	return sessionpkg.ComputeKey(tenantID, req.Conversation, sessionpkg.DefaultScope)
}
