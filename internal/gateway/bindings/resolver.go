package bindings

import (
	"context"
	"fmt"

	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
)

const payloadTypeNewSession = "new_session"

type DefaultResolver struct {
	tenantID string
	repo     SessionLookup
}

func NewResolver(tenantID string, repo SessionLookup) *DefaultResolver {
	return &DefaultResolver{tenantID: tenantID, repo: repo}
}

func (r *DefaultResolver) Resolve(ctx context.Context, in gatewaymodel.NormalizedIngress) (gatewaymodel.BindingContext, error) {
	scope, metadata, sessionID, err := r.resolveScope(ctx, in)
	if err != nil {
		return gatewaymodel.BindingContext{}, err
	}
	sessionKey, err := ComputeKey(r.tenantID, in.Conversation, scope)
	if err != nil {
		return gatewaymodel.BindingContext{}, err
	}
	return gatewaymodel.BindingContext{
		TenantID:     r.tenantID,
		SessionKey:   sessionKey,
		SessionID:    sessionID,
		Scope:        scope,
		Conversation: in.Conversation,
		Metadata:     metadata,
	}, nil
}

func (r *DefaultResolver) resolveScope(ctx context.Context, in gatewaymodel.NormalizedIngress) (string, map[string]string, string, error) {
	if in.Payload.Type == "message" && IsNewSessionCommand(in.Payload.Text) {
		meta := WithHint(nil, HintPayloadTypeOverride, payloadTypeNewSession)
		meta = WithHint(meta, HintScopeSource, ScopeSourceNewSession)
		return NewScopeFromID(in.IdempotencyKey), meta, "", nil
	}

	if scope, sessionID, ok, err := r.scopeFromSessionHint(ctx, in); err != nil {
		return "", nil, "", err
	} else if ok {
		meta := WithHint(nil, HintScopeSource, ScopeSourceSessionKey)
		return scope, meta, sessionID, nil
	}

	if in.DMScope != "" {
		meta := WithHint(nil, HintScopeSource, ScopeSourceIngress)
		return ScopeFromIngress(in), meta, "", nil
	}

	if r.repo != nil {
		scope, ok, err := r.repo.GetConversationDMScope(ctx, r.tenantID, in.Conversation)
		if err != nil {
			return "", nil, "", fmt.Errorf("load conversation scope: %w", err)
		}
		if ok {
			meta := WithHint(nil, HintScopeSource, ScopeSourceStored)
			return NormalizeScope(scope), meta, "", nil
		}
	}

	meta := WithHint(nil, HintScopeSource, ScopeSourceDefault)
	return DefaultScope, meta, "", nil
}

func (r *DefaultResolver) scopeFromSessionHint(ctx context.Context, in gatewaymodel.NormalizedIngress) (string, string, bool, error) {
	if r.repo == nil || in.SessionKeyHint == "" {
		return "", "", false, nil
	}
	rec, ok, err := r.repo.GetScopeSession(ctx, in.SessionKeyHint)
	if err != nil {
		return "", "", false, fmt.Errorf("load session hint: %w", err)
	}
	if !ok {
		return "", "", false, nil
	}
	if rec.ConversationID != in.Conversation.ConversationID ||
		rec.ChannelType != in.Conversation.ChannelType ||
		rec.ParticipantID != in.Conversation.ParticipantID {
		return "", "", false, nil
	}
	return NormalizeScope(rec.DMScope), rec.SessionID, true, nil
}
