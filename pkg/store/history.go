package store

import (
	"context"
	"strings"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (db *DB) RecentMessages(ctx context.Context, sessionID string, limit int) ([]HistoryMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.reader.QueryContext(
		ctx,
		`SELECT role, content, tool_call_id, tool_name
		 FROM messages
		 WHERE session_id = ?
		 ORDER BY created_at DESC, fts_rowid DESC
		 LIMIT ?`,
		sessionID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryMessage
	for rows.Next() {
		var msg HistoryMessage
		if err := rows.Scan(&msg.Role, &msg.Content, &msg.ToolCallID, &msg.ToolName); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	reverseHistory(out)
	return out, nil
}

func (db *DB) SearchMessagesFTS(ctx context.Context, sessionID, query string, limit int) ([]model.RAGHit, error) {
	if limit <= 0 {
		limit = 5
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	rows, err := db.reader.QueryContext(
		ctx,
		`SELECT m.content
		 FROM messages_fts f
		 JOIN messages m ON m.fts_rowid = f.rowid
		 WHERE m.session_id = ? AND f.content MATCH ?
		 LIMIT ?`,
		sessionID,
		query,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []model.RAGHit
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		hits = append(hits, model.RAGHit{
			Path:    sessionID,
			Scope:   "session",
			Preview: content,
		})
	}
	return hits, rows.Err()
}

func reverseHistory(items []HistoryMessage) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}
