package gateway

import (
	"context"
	"net/http"

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
