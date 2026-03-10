package ingest

import (
	"context"
	"github.com/similarityyoung/simiclaw/internal/readmodel"
	"github.com/similarityyoung/simiclaw/pkg/api"

	"github.com/similarityyoung/simiclaw/internal/session"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const payloadTypeNewSession = "new_session"

type SessionReader interface {
	GetConversationDMScope(ctx context.Context, tenantID string, conv model.Conversation) (string, bool, error)
	GetSession(ctx context.Context, sessionKey string) (readmodel.SessionRecord, bool, error)
}

type DefaultScopeResolver struct {
	tenantID string
	repo     SessionReader
}

func NewScopeResolver(tenantID string, repo SessionReader) *DefaultScopeResolver {
	return &DefaultScopeResolver{tenantID: tenantID, repo: repo}
}

func (r *DefaultScopeResolver) Resolve(ctx context.Context, req api.IngestRequest) (api.IngestRequest, string, *Error) {
	if req.Payload.Type == "message" && session.IsNewSessionCommand(req.Payload.Text) {
		scope := session.NewScopeFromID(req.IdempotencyKey)
		req.DMScope = scope
		req.Payload.Type = payloadTypeNewSession
		return req, scope, nil
	}

	if scope, ok, err := r.scopeFromSessionHint(ctx, req); err != nil {
		return api.IngestRequest{}, "", err
	} else if ok {
		req.DMScope = scope
		return req, scope, nil
	}

	if req.DMScope != "" {
		scope := session.ScopeFromRequest(req)
		req.DMScope = scope
		return req, scope, nil
	}

	scope, ok, err := r.repo.GetConversationDMScope(ctx, r.tenantID, req.Conversation)
	if err != nil {
		return api.IngestRequest{}, "", &Error{
			Code:    model.ErrorCodeInternal,
			Message: err.Error(),
		}
	}
	if !ok {
		scope = session.DefaultScope
	}
	req.DMScope = scope
	return req, scope, nil
}

func (r *DefaultScopeResolver) scopeFromSessionHint(ctx context.Context, req api.IngestRequest) (string, bool, *Error) {
	if req.SessionKeyHint == "" {
		return "", false, nil
	}
	rec, ok, err := r.repo.GetSession(ctx, req.SessionKeyHint)
	if err != nil {
		return "", false, &Error{
			Code:    model.ErrorCodeInternal,
			Message: err.Error(),
		}
	}
	if !ok {
		return "", false, nil
	}
	if rec.ConversationID != req.Conversation.ConversationID || rec.ChannelType != req.Conversation.ChannelType || rec.ParticipantID != req.Conversation.ParticipantID {
		return "", false, nil
	}
	return session.NormalizeScope(rec.DMScope), true, nil
}
