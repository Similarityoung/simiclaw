package runtime

import (
	"context"

	gatewaybindings "github.com/similarityyoung/simiclaw/internal/gateway/bindings"
	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type testEventRecordQuery interface {
	GetEventRecord(ctx context.Context, eventID string) (querymodel.EventRecord, bool, error)
}

type testConversationScopeQuery interface {
	GetConversationDMScope(ctx context.Context, tenantID string, conv model.Conversation) (string, bool, error)
}

type testSessionRecordQuery interface {
	GetSessionRecord(ctx context.Context, sessionKey string) (querymodel.SessionRecord, bool, error)
}

type testQueryEventView struct {
	query testEventRecordQuery
}

func (v testQueryEventView) GetEventRecord(ctx context.Context, eventID string) (runtimemodel.EventRecord, bool, error) {
	if v.query == nil {
		return runtimemodel.EventRecord{}, false, nil
	}
	rec, ok, err := v.query.GetEventRecord(ctx, eventID)
	if err != nil || !ok {
		return runtimemodel.EventRecord{}, ok, err
	}
	return runtimemodel.EventRecord{
		EventID:           rec.EventID,
		Status:            rec.Status,
		OutboxStatus:      rec.OutboxStatus,
		SessionKey:        rec.SessionKey,
		SessionID:         rec.SessionID,
		RunID:             rec.RunID,
		RunMode:           rec.RunMode,
		AssistantReply:    rec.AssistantReply,
		OutboxID:          rec.OutboxID,
		ProcessingLease:   rec.ProcessingLease,
		ReceivedAt:        rec.ReceivedAt,
		CreatedAt:         rec.CreatedAt,
		UpdatedAt:         rec.UpdatedAt,
		PayloadHash:       rec.PayloadHash,
		Provider:          rec.Provider,
		Model:             rec.Model,
		ProviderRequestID: rec.ProviderRequestID,
		Error:             rec.Error,
	}, true, nil
}

type testGatewaySessionLookup struct {
	scopes   testConversationScopeQuery
	sessions testSessionRecordQuery
}

func newTestGatewaySessionLookup(db *store.DB, query testSessionRecordQuery) testGatewaySessionLookup {
	return testGatewaySessionLookup{scopes: db, sessions: query}
}

func (l testGatewaySessionLookup) GetConversationDMScope(ctx context.Context, tenantID string, conv model.Conversation) (string, bool, error) {
	if l.scopes == nil {
		return "", false, nil
	}
	return l.scopes.GetConversationDMScope(ctx, tenantID, conv)
}

func (l testGatewaySessionLookup) GetScopeSession(ctx context.Context, sessionKey string) (gatewaybindings.SessionScopeRecord, bool, error) {
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
