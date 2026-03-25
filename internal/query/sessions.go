package query

import (
	"context"

	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
)

func (s *Service) GetSession(ctx context.Context, sessionKey string) (querymodel.SessionRecord, bool, error) {
	return s.sessions.GetSessionRecord(ctx, sessionKey)
}

func (s *Service) ListSessionHistory(ctx context.Context, filter querymodel.SessionHistoryFilter) (querymodel.MessagePage, error) {
	filter.Limit = pageFetchLimit(filter.Limit)
	items, err := s.sessions.ListMessageRecords(ctx, filter)
	if err != nil {
		return querymodel.MessagePage{}, err
	}
	return buildMessagePage(items, filter.Limit-1), nil
}

func (s *Service) ListSessions(ctx context.Context, filter querymodel.SessionFilter) (querymodel.SessionPage, error) {
	filter.Limit = pageFetchLimit(filter.Limit)
	items, err := s.sessions.ListSessionRecords(ctx, filter)
	if err != nil {
		return querymodel.SessionPage{}, err
	}
	return buildSessionPage(items, filter.Limit-1), nil
}

func buildSessionPage(items []querymodel.SessionRecord, limit int) querymodel.SessionPage {
	if limit <= 0 || len(items) <= limit {
		return querymodel.SessionPage{Items: items}
	}
	trimmed := items[:limit]
	last := trimmed[len(trimmed)-1]
	return querymodel.SessionPage{
		Items: trimmed,
		Next: &querymodel.SessionCursorAnchor{
			LastActivityAt: last.LastActivityAt,
			SessionKey:     last.SessionKey,
		},
	}
}

func buildMessagePage(items []querymodel.MessageRecord, limit int) querymodel.MessagePage {
	if limit <= 0 || len(items) <= limit {
		return querymodel.MessagePage{Items: items}
	}
	trimmed := items[1:]
	first := trimmed[0]
	return querymodel.MessagePage{
		Items: trimmed,
		Next: &querymodel.MessageCursorAnchor{
			CreatedAt: first.CreatedAt,
			MessageID: first.MessageID,
		},
	}
}
