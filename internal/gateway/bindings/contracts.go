package bindings

import (
	"context"

	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type SessionScopeRecord struct {
	ConversationID string
	ChannelType    string
	ParticipantID  string
	DMScope        string
	SessionID      string
}

type SessionLookup interface {
	GetConversationDMScope(ctx context.Context, tenantID string, conv model.Conversation) (string, bool, error)
	GetScopeSession(ctx context.Context, sessionKey string) (SessionScopeRecord, bool, error)
}

// Resolver computes binding state such as session key, scope, and hints for a
// normalized ingress request before runtime routing begins.
type Resolver interface {
	Resolve(ctx context.Context, in gatewaymodel.NormalizedIngress) (gatewaymodel.BindingContext, error)
}
