package query

import (
	"context"
	"time"

	"github.com/similarityyoung/simiclaw/internal/readmodel"
	"github.com/similarityyoung/simiclaw/internal/store"
)

func (s *Service) GetSession(ctx context.Context, sessionKey string) (readmodel.SessionRecord, bool, error) {
	return s.repo.GetSession(ctx, sessionKey)
}

func (s *Service) ListSessionHistory(ctx context.Context, query SessionHistoryQuery) (MessagePage, error) {
	before, beforeID := messageCursorFilter(query.Cursor)
	items, err := s.repo.ListMessages(ctx, query.SessionID, pageFetchLimit(query.Limit), before, beforeID, query.VisibleOnly)
	if err != nil {
		return MessagePage{}, err
	}
	return buildMessagePage(items, query.Limit), nil
}

func (s *Service) ListSessions(ctx context.Context, query SessionListQuery) (SessionPage, error) {
	filter := store.SessionListFilter{
		SessionKey:     query.SessionKey,
		ConversationID: query.ConversationID,
		Limit:          pageFetchLimit(query.Limit),
	}
	if query.Cursor != nil {
		filter.CursorLastActivityAt = query.Cursor.LastActivityAt
		filter.CursorLastSessionKey = query.Cursor.SessionKey
	}
	items, err := s.repo.ListSessionsPage(ctx, filter)
	if err != nil {
		return SessionPage{}, err
	}
	return buildSessionPage(items, query.Limit), nil
}

func buildSessionPage(items []readmodel.SessionRecord, limit int) SessionPage {
	if limit <= 0 || len(items) <= limit {
		return SessionPage{Items: items}
	}
	trimmed := items[:limit]
	last := trimmed[len(trimmed)-1]
	return SessionPage{
		Items: trimmed,
		Next: &SessionCursorAnchor{
			LastActivityAt: last.LastActivityAt,
			SessionKey:     last.SessionKey,
		},
	}
}

func buildMessagePage(items []readmodel.MessageRecord, limit int) MessagePage {
	if limit <= 0 || len(items) <= limit {
		return MessagePage{Items: items}
	}
	trimmed := items[1:]
	first := trimmed[0]
	return MessagePage{
		Items: trimmed,
		Next: &MessageCursorAnchor{
			CreatedAt: first.CreatedAt,
			MessageID: first.MessageID,
		},
	}
}

func messageCursorFilter(cursor *MessageCursorAnchor) (time.Time, string) {
	if cursor == nil {
		return time.Time{}, ""
	}
	return cursor.CreatedAt, cursor.MessageID
}
