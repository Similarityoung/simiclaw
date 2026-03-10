package query

import (
	"context"
	"time"

	"github.com/similarityyoung/simiclaw/internal/readmodel"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type EventCursorAnchor struct {
	CreatedAt time.Time
	EventID   string
}

type RunCursorAnchor struct {
	StartedAt time.Time
	RunID     string
}

type SessionCursorAnchor struct {
	LastActivityAt time.Time
	SessionKey     string
}

type MessageCursorAnchor struct {
	CreatedAt time.Time
	MessageID string
}

type EventListQuery struct {
	SessionKey string
	Status     model.EventStatus
	Limit      int
	Cursor     *EventCursorAnchor
}

type RunListQuery struct {
	SessionKey string
	SessionID  string
	Limit      int
	Cursor     *RunCursorAnchor
}

type SessionListQuery struct {
	SessionKey     string
	ConversationID string
	Limit          int
	Cursor         *SessionCursorAnchor
}

type SessionHistoryQuery struct {
	SessionID   string
	VisibleOnly bool
	Limit       int
	Cursor      *MessageCursorAnchor
}

type EventPage struct {
	Items []readmodel.EventRecord
	Next  *EventCursorAnchor
}

type RunPage struct {
	Items []api.RunTrace
	Next  *RunCursorAnchor
}

type SessionPage struct {
	Items []readmodel.SessionRecord
	Next  *SessionCursorAnchor
}

type MessagePage struct {
	Items []readmodel.MessageRecord
	Next  *MessageCursorAnchor
}

type Repository interface {
	GetEvent(ctx context.Context, eventID string) (readmodel.EventRecord, bool, error)
	LookupInbound(ctx context.Context, key string) (readmodel.LookupEvent, bool, error)
	GetRun(ctx context.Context, runID string) (api.RunTrace, bool, error)
	GetSession(ctx context.Context, sessionKey string) (readmodel.SessionRecord, bool, error)
	ListMessages(ctx context.Context, sessionID string, limit int, before time.Time, beforeMessageID string, visibleOnly bool) ([]readmodel.MessageRecord, error)
	ListEventsPage(ctx context.Context, filter store.EventListFilter) ([]readmodel.EventRecord, error)
	ListRunsPage(ctx context.Context, filter store.RunListFilter) ([]api.RunTrace, error)
	ListSessionsPage(ctx context.Context, filter store.SessionListFilter) ([]readmodel.SessionRecord, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func pageFetchLimit(limit int) int {
	if limit <= 0 {
		return 51
	}
	return limit + 1
}
