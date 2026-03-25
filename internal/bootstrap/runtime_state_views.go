package bootstrap

import (
	"context"

	gatewaybindings "github.com/similarityyoung/simiclaw/internal/gateway/bindings"
	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type eventRecordQuery interface {
	GetEventRecord(ctx context.Context, eventID string) (querymodel.EventRecord, bool, error)
}

type conversationScopeQuery interface {
	GetConversationDMScope(ctx context.Context, tenantID string, conv model.Conversation) (string, bool, error)
}

type sessionRecordQuery interface {
	GetSessionRecord(ctx context.Context, sessionKey string) (querymodel.SessionRecord, bool, error)
}

type runtimeEventRecordView struct {
	query eventRecordQuery
}

func (v runtimeEventRecordView) GetEventRecord(ctx context.Context, eventID string) (runtimemodel.EventRecord, bool, error) {
	if v.query == nil {
		return runtimemodel.EventRecord{}, false, nil
	}
	rec, ok, err := v.query.GetEventRecord(ctx, eventID)
	if err != nil || !ok {
		return runtimemodel.EventRecord{}, ok, err
	}
	return queryEventRecordToRuntime(rec), true, nil
}

type gatewaySessionLookup struct {
	scopes   conversationScopeQuery
	sessions sessionRecordQuery
}

func (l gatewaySessionLookup) GetConversationDMScope(ctx context.Context, tenantID string, conv model.Conversation) (string, bool, error) {
	if l.scopes == nil {
		return "", false, nil
	}
	return l.scopes.GetConversationDMScope(ctx, tenantID, conv)
}

func (l gatewaySessionLookup) GetScopeSession(ctx context.Context, sessionKey string) (gatewaybindings.SessionScopeRecord, bool, error) {
	if l.sessions == nil {
		return gatewaybindings.SessionScopeRecord{}, false, nil
	}
	rec, ok, err := l.sessions.GetSessionRecord(ctx, sessionKey)
	if err != nil || !ok {
		return gatewaybindings.SessionScopeRecord{}, ok, err
	}
	return gatewaybindings.SessionScopeRecord{
		ConversationID: rec.ConversationID,
		ChannelType:    rec.ChannelType,
		ParticipantID:  rec.ParticipantID,
		DMScope:        rec.DMScope,
		SessionID:      rec.ActiveSessionID,
	}, true, nil
}
