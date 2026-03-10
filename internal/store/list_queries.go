package store

import (
	"context"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/internal/readmodel"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

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

func (db *DB) ListEventsPage(ctx context.Context, filter EventListFilter) ([]readmodel.EventRecord, error) {
	query := eventSelectSQL + ` WHERE 1 = 1`
	args := make([]any, 0, 6)
	if strings.TrimSpace(filter.SessionKey) != "" {
		query += ` AND session_key = ?`
		args = append(args, filter.SessionKey)
	}
	if filter.Status != "" {
		query += ` AND status = ?`
		args = append(args, string(filter.Status))
	}
	if !filter.CursorCreatedAt.IsZero() && strings.TrimSpace(filter.CursorEventID) != "" {
		cursor := timeText(filter.CursorCreatedAt)
		query += ` AND (created_at < ? OR (created_at = ? AND event_id < ?))`
		args = append(args, cursor, cursor, filter.CursorEventID)
	}
	query += ` ORDER BY created_at DESC, event_id DESC LIMIT ?`
	args = append(args, normalizeListLimit(filter.Limit))

	rows, err := db.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []readmodel.EventRecord
	for rows.Next() {
		rec, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (db *DB) ListRunsPage(ctx context.Context, filter RunListFilter) ([]api.RunTrace, error) {
	query := runSelectSQL + ` WHERE 1 = 1`
	args := make([]any, 0, 6)
	if strings.TrimSpace(filter.SessionKey) != "" {
		query += ` AND session_key = ?`
		args = append(args, filter.SessionKey)
	}
	if strings.TrimSpace(filter.SessionID) != "" {
		query += ` AND session_id = ?`
		args = append(args, filter.SessionID)
	}
	if !filter.CursorStartedAt.IsZero() && strings.TrimSpace(filter.CursorRunID) != "" {
		cursor := timeText(filter.CursorStartedAt)
		query += ` AND (started_at < ? OR (started_at = ? AND run_id < ?))`
		args = append(args, cursor, cursor, filter.CursorRunID)
	}
	query += ` ORDER BY started_at DESC, run_id DESC LIMIT ?`
	args = append(args, normalizeListLimit(filter.Limit))

	rows, err := db.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []api.RunTrace
	for rows.Next() {
		trace, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, trace)
	}
	return out, rows.Err()
}

func (db *DB) ListSessionsPage(ctx context.Context, filter SessionListFilter) ([]readmodel.SessionRecord, error) {
	query := sessionSelectSQL + ` WHERE 1 = 1`
	args := make([]any, 0, 6)
	if strings.TrimSpace(filter.SessionKey) != "" {
		query += ` AND session_key = ?`
		args = append(args, filter.SessionKey)
	}
	if strings.TrimSpace(filter.ConversationID) != "" {
		query += ` AND conversation_id = ?`
		args = append(args, filter.ConversationID)
	}
	if !filter.CursorLastActivityAt.IsZero() && strings.TrimSpace(filter.CursorLastSessionKey) != "" {
		cursor := timeText(filter.CursorLastActivityAt)
		query += ` AND (last_activity_at < ? OR (last_activity_at = ? AND session_key < ?))`
		args = append(args, cursor, cursor, filter.CursorLastSessionKey)
	}
	query += ` ORDER BY last_activity_at DESC, session_key DESC LIMIT ?`
	args = append(args, normalizeListLimit(filter.Limit))

	rows, err := db.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []readmodel.SessionRecord
	for rows.Next() {
		rec, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func normalizeListLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	return limit
}
