package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
	runnermodel "github.com/similarityyoung/simiclaw/internal/runner/model"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	storepkg "github.com/similarityyoung/simiclaw/internal/store"
	storequeries "github.com/similarityyoung/simiclaw/internal/store/queries"
	storetx "github.com/similarityyoung/simiclaw/internal/store/tx"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

var (
	DefaultBusyTimeout = storepkg.DefaultBusyTimeout
	InitWorkspace      = storepkg.InitWorkspace
	Open               = storepkg.Open
	DBPath             = storepkg.DBPath
)

type ClaimedJob = storepkg.ClaimedJob
type ScheduledJobPayload = storepkg.ScheduledJobPayload
type StoredMessage = runtimemodel.StoredMessage
type RunFinalize = runtimemodel.RunFinalize
type ClaimedOutbox = runtimemodel.ClaimedOutbox
type HistoryMessage = runnermodel.HistoryMessage

type ClaimedEvent struct {
	Event   model.InternalEvent
	RunID   string
	Status  model.EventStatus
	RunMode model.RunMode
}

type EventListFilter struct {
	SessionKey      string
	Status          model.EventStatus
	Limit           int
	CursorCreatedAt time.Time
	CursorEventID   string
}

type RunListFilter struct {
	SessionKey      string
	SessionID       string
	Limit           int
	CursorStartedAt time.Time
	CursorRunID     string
}

type SessionListFilter struct {
	SessionKey           string
	ConversationID       string
	Limit                int
	CursorLastActivityAt time.Time
	CursorLastSessionKey string
}

type runtimeDB struct {
	*storepkg.DB
	runtime *storetx.RuntimeRepository
	queries *storequeries.Repository
}

func newTestDB(t *testing.T) *runtimeDB {
	t.Helper()
	workspace := t.TempDir()
	if err := storepkg.InitWorkspace(workspace, false, storepkg.DefaultBusyTimeout()); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	db, err := storepkg.Open(workspace, storepkg.DefaultBusyTimeout())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return &runtimeDB{
		DB:      db,
		runtime: storetx.NewRuntimeRepository(db),
		queries: storequeries.NewRepository(db),
	}
}

func (db *runtimeDB) IngestEvent(ctx context.Context, tenantID, sessionKey string, req gateway.PersistRequest, payloadHash string, now time.Time) (gateway.PersistResult, error) {
	return db.runtime.PersistEvent(ctx, tenantID, sessionKey, req, payloadHash, now)
}

func (db *runtimeDB) MarkEventQueued(ctx context.Context, eventID string, now time.Time) error {
	return db.runtime.MarkEventQueued(ctx, eventID, now)
}

func (db *runtimeDB) LookupInbound(ctx context.Context, key string) (querymodel.LookupEvent, bool, error) {
	return db.queries.LookupEvent(ctx, key)
}

func (db *runtimeDB) GetEvent(ctx context.Context, eventID string) (querymodel.EventRecord, bool, error) {
	return db.queries.GetEventRecord(ctx, eventID)
}

func (db *runtimeDB) ListRunnableEventIDs(ctx context.Context, limit int) ([]string, error) {
	items, err := db.runtime.ListRunnable(ctx, limit)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.EventID)
	}
	return ids, nil
}

func (db *runtimeDB) ClaimEvent(ctx context.Context, eventID, runID string, now time.Time) (ClaimedEvent, bool, error) {
	claim, ok, err := db.runtime.ClaimWork(ctx, runtimemodel.WorkItem{
		EventID: eventID,
	}, runID, now)
	if err != nil || !ok {
		return ClaimedEvent{}, ok, err
	}
	return ClaimedEvent{
		Event:   claim.Event,
		RunID:   claim.RunID,
		Status:  model.EventStatusProcessing,
		RunMode: claim.RunMode,
	}, true, nil
}

func (db *runtimeDB) FinalizeRun(ctx context.Context, finalize RunFinalize) error {
	return db.runtime.Finalize(ctx, runtimemodel.FinalizeCommand(finalize))
}

func (db *runtimeDB) GetRun(ctx context.Context, runID string) (querymodel.RunTrace, bool, error) {
	return db.queries.GetRunTrace(ctx, runID)
}

func (db *runtimeDB) GetSession(ctx context.Context, sessionKey string) (querymodel.SessionRecord, bool, error) {
	return db.queries.GetSessionRecord(ctx, sessionKey)
}

func (db *runtimeDB) ListEventsPage(ctx context.Context, filter EventListFilter) ([]querymodel.EventRecord, error) {
	var cursor *querymodel.EventCursorAnchor
	if !filter.CursorCreatedAt.IsZero() || strings.TrimSpace(filter.CursorEventID) != "" {
		cursor = &querymodel.EventCursorAnchor{
			CreatedAt: filter.CursorCreatedAt,
			EventID:   filter.CursorEventID,
		}
	}
	return db.queries.ListEventRecords(ctx, querymodel.EventFilter{
		SessionKey: filter.SessionKey,
		Status:     filter.Status,
		Limit:      filter.Limit,
		Cursor:     cursor,
	})
}

func (db *runtimeDB) ListRunsPage(ctx context.Context, filter RunListFilter) ([]querymodel.RunTrace, error) {
	var cursor *querymodel.RunCursorAnchor
	if !filter.CursorStartedAt.IsZero() || strings.TrimSpace(filter.CursorRunID) != "" {
		cursor = &querymodel.RunCursorAnchor{
			StartedAt: filter.CursorStartedAt,
			RunID:     filter.CursorRunID,
		}
	}
	return db.queries.ListRunTraces(ctx, querymodel.RunFilter{
		SessionKey: filter.SessionKey,
		SessionID:  filter.SessionID,
		Limit:      filter.Limit,
		Cursor:     cursor,
	})
}

func (db *runtimeDB) ListSessionsPage(ctx context.Context, filter SessionListFilter) ([]querymodel.SessionRecord, error) {
	var cursor *querymodel.SessionCursorAnchor
	if !filter.CursorLastActivityAt.IsZero() || strings.TrimSpace(filter.CursorLastSessionKey) != "" {
		cursor = &querymodel.SessionCursorAnchor{
			LastActivityAt: filter.CursorLastActivityAt,
			SessionKey:     filter.CursorLastSessionKey,
		}
	}
	return db.queries.ListSessionRecords(ctx, querymodel.SessionFilter{
		SessionKey:     filter.SessionKey,
		ConversationID: filter.ConversationID,
		Limit:          filter.Limit,
		Cursor:         cursor,
	})
}

func (db *runtimeDB) ListMessages(ctx context.Context, sessionID string, limit int, before time.Time, beforeMessageID string, visibleOnly bool) ([]querymodel.MessageRecord, error) {
	var cursor *querymodel.MessageCursorAnchor
	if !before.IsZero() || strings.TrimSpace(beforeMessageID) != "" {
		cursor = &querymodel.MessageCursorAnchor{
			CreatedAt: before,
			MessageID: beforeMessageID,
		}
	}
	return db.queries.ListMessageRecords(ctx, querymodel.SessionHistoryFilter{
		SessionID:   sessionID,
		VisibleOnly: visibleOnly,
		Limit:       limit,
		Cursor:      cursor,
	})
}

func (db *runtimeDB) RecentMessages(ctx context.Context, sessionID string, limit int) ([]HistoryMessage, error) {
	return db.queries.LoadRecentHistory(ctx, sessionID, limit)
}

func (db *runtimeDB) RecentMessagesForPrompt(ctx context.Context, sessionID string, limit int) ([]HistoryMessage, error) {
	return db.queries.LoadPromptHistory(ctx, sessionID, limit)
}

func (db *runtimeDB) SearchMessagesFTS(ctx context.Context, sessionID, query string, limit int) ([]runnermodel.RAGHit, error) {
	return db.queries.SearchRAGHits(ctx, sessionID, query, limit)
}

func (db *runtimeDB) RecoverExpiredSending(ctx context.Context, cutoff, now time.Time) error {
	return db.runtime.RecoverExpiredSending(ctx, cutoff, now)
}

func (db *runtimeDB) RecoverExpiredProcessing(ctx context.Context, cutoff, now time.Time) ([]string, error) {
	return db.runtime.RecoverExpiredProcessing(ctx, cutoff, now)
}

func (db *runtimeDB) ClaimOutbox(ctx context.Context, owner string, now time.Time) (ClaimedOutbox, bool, error) {
	return db.runtime.ClaimRuntimeOutbox(ctx, owner, now)
}

func (db *runtimeDB) CompleteOutboxSend(ctx context.Context, outboxID, eventID string, now time.Time) error {
	return db.runtime.CompleteOutboxSend(ctx, outboxID, eventID, now)
}

func (db *runtimeDB) FailOutboxSend(ctx context.Context, outboxID, eventID, message string, dead bool, nextAttemptAt, now time.Time) error {
	return db.runtime.FailOutboxSend(ctx, outboxID, eventID, message, dead, nextAttemptAt, now)
}

func persistRequest(req api.IngestRequest) gateway.PersistRequest {
	return gateway.PersistRequest{
		Source:         req.Source,
		Conversation:   req.Conversation,
		Payload:        req.Payload,
		IdempotencyKey: req.IdempotencyKey,
		DMScope:        req.DMScope,
	}
}

func timeText(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func mustParseTime(raw string) time.Time {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func normalizeListLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	return limit
}

func conversationParticipantID(conv model.Conversation) string {
	if conv.ChannelType == "dm" {
		return conv.ParticipantID
	}
	return ""
}
