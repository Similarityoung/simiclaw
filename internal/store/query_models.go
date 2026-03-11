package store

import (
	"context"
	"time"

	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
	"github.com/similarityyoung/simiclaw/internal/readmodel"
)

func (db *DB) GetEventRecord(ctx context.Context, eventID string) (querymodel.EventRecord, bool, error) {
	rec, ok, err := db.GetEvent(ctx, eventID)
	if err != nil || !ok {
		return querymodel.EventRecord{}, ok, err
	}
	return toQueryEventRecord(rec), true, nil
}

func (db *DB) LookupEvent(ctx context.Context, key string) (querymodel.LookupEvent, bool, error) {
	row, ok, err := db.LookupInbound(ctx, key)
	if err != nil || !ok {
		return querymodel.LookupEvent{}, ok, err
	}
	return toQueryLookupEvent(row), true, nil
}

func (db *DB) GetRunTrace(ctx context.Context, runID string) (querymodel.RunTrace, bool, error) {
	trace, ok, err := db.GetRun(ctx, runID)
	return trace, ok, err
}

func (db *DB) GetSessionRecord(ctx context.Context, sessionKey string) (querymodel.SessionRecord, bool, error) {
	rec, ok, err := db.GetSession(ctx, sessionKey)
	if err != nil || !ok {
		return querymodel.SessionRecord{}, ok, err
	}
	return toQuerySessionRecord(rec), true, nil
}

func (db *DB) ListMessageRecords(ctx context.Context, filter querymodel.SessionHistoryFilter) ([]querymodel.MessageRecord, error) {
	before, beforeMessageID := queryHistoryCursor(filter.Cursor)
	items, err := db.ListMessages(ctx, filter.SessionID, filter.Limit, before, beforeMessageID, filter.VisibleOnly)
	if err != nil {
		return nil, err
	}
	return mapMessageRecords(items), nil
}

func (db *DB) ListEventRecords(ctx context.Context, filter querymodel.EventFilter) ([]querymodel.EventRecord, error) {
	items, err := db.ListEventsPage(ctx, EventListFilter{
		SessionKey:      filter.SessionKey,
		Status:          filter.Status,
		Limit:           filter.Limit,
		CursorCreatedAt: queryEventCreatedAt(filter.Cursor),
		CursorEventID:   queryEventID(filter.Cursor),
	})
	if err != nil {
		return nil, err
	}
	return mapEventRecords(items), nil
}

func (db *DB) ListRunTraces(ctx context.Context, filter querymodel.RunFilter) ([]querymodel.RunTrace, error) {
	return db.ListRunsPage(ctx, RunListFilter{
		SessionKey:      filter.SessionKey,
		SessionID:       filter.SessionID,
		Limit:           filter.Limit,
		CursorStartedAt: queryRunStartedAt(filter.Cursor),
		CursorRunID:     queryRunID(filter.Cursor),
	})
}

func (db *DB) ListSessionRecords(ctx context.Context, filter querymodel.SessionFilter) ([]querymodel.SessionRecord, error) {
	items, err := db.ListSessionsPage(ctx, SessionListFilter{
		SessionKey:           filter.SessionKey,
		ConversationID:       filter.ConversationID,
		Limit:                filter.Limit,
		CursorLastActivityAt: querySessionLastActivityAt(filter.Cursor),
		CursorLastSessionKey: querySessionKey(filter.Cursor),
	})
	if err != nil {
		return nil, err
	}
	return mapSessionRecords(items), nil
}

func mapEventRecords(items []readmodel.EventRecord) []querymodel.EventRecord {
	out := make([]querymodel.EventRecord, 0, len(items))
	for _, item := range items {
		out = append(out, toQueryEventRecord(item))
	}
	return out
}

func mapSessionRecords(items []readmodel.SessionRecord) []querymodel.SessionRecord {
	out := make([]querymodel.SessionRecord, 0, len(items))
	for _, item := range items {
		out = append(out, toQuerySessionRecord(item))
	}
	return out
}

func mapMessageRecords(items []readmodel.MessageRecord) []querymodel.MessageRecord {
	out := make([]querymodel.MessageRecord, 0, len(items))
	for _, item := range items {
		out = append(out, toQueryMessageRecord(item))
	}
	return out
}

func toQueryEventRecord(rec readmodel.EventRecord) querymodel.EventRecord {
	return querymodel.EventRecord{
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
	}
}

func toQueryLookupEvent(row readmodel.LookupEvent) querymodel.LookupEvent {
	return querymodel.LookupEvent{
		EventID:     row.EventID,
		PayloadHash: row.PayloadHash,
		ReceivedAt:  row.ReceivedAt,
		SessionKey:  row.SessionKey,
		SessionID:   row.SessionID,
	}
}

func toQuerySessionRecord(rec readmodel.SessionRecord) querymodel.SessionRecord {
	return querymodel.SessionRecord{
		SessionKey:            rec.SessionKey,
		ActiveSessionID:       rec.ActiveSessionID,
		ConversationID:        rec.ConversationID,
		ChannelType:           rec.ChannelType,
		ParticipantID:         rec.ParticipantID,
		DMScope:               rec.DMScope,
		MessageCount:          rec.MessageCount,
		PromptTokensTotal:     rec.PromptTokensTotal,
		CompletionTokensTotal: rec.CompletionTokensTotal,
		TotalTokensTotal:      rec.TotalTokensTotal,
		LastModel:             rec.LastModel,
		LastRunID:             rec.LastRunID,
		LastActivityAt:        rec.LastActivityAt,
		CreatedAt:             rec.CreatedAt,
		UpdatedAt:             rec.UpdatedAt,
	}
}

func toQueryMessageRecord(rec readmodel.MessageRecord) querymodel.MessageRecord {
	return querymodel.MessageRecord{
		MessageID:  rec.MessageID,
		SessionKey: rec.SessionKey,
		SessionID:  rec.SessionID,
		RunID:      rec.RunID,
		Role:       rec.Role,
		Content:    rec.Content,
		Visible:    rec.Visible,
		ToolCallID: rec.ToolCallID,
		ToolName:   rec.ToolName,
		ToolArgs:   rec.ToolArgs,
		ToolResult: rec.ToolResult,
		Meta:       rec.Meta,
		CreatedAt:  rec.CreatedAt,
	}
}

func queryHistoryCursor(cursor *querymodel.MessageCursorAnchor) (time.Time, string) {
	if cursor == nil {
		return time.Time{}, ""
	}
	return cursor.CreatedAt, cursor.MessageID
}

func queryEventCreatedAt(cursor *querymodel.EventCursorAnchor) time.Time {
	if cursor == nil {
		return time.Time{}
	}
	return cursor.CreatedAt
}

func queryEventID(cursor *querymodel.EventCursorAnchor) string {
	if cursor == nil {
		return ""
	}
	return cursor.EventID
}

func queryRunStartedAt(cursor *querymodel.RunCursorAnchor) time.Time {
	if cursor == nil {
		return time.Time{}
	}
	return cursor.StartedAt
}

func queryRunID(cursor *querymodel.RunCursorAnchor) string {
	if cursor == nil {
		return ""
	}
	return cursor.RunID
}

func querySessionLastActivityAt(cursor *querymodel.SessionCursorAnchor) time.Time {
	if cursor == nil {
		return time.Time{}
	}
	return cursor.LastActivityAt
}

func querySessionKey(cursor *querymodel.SessionCursorAnchor) string {
	if cursor == nil {
		return ""
	}
	return cursor.SessionKey
}
