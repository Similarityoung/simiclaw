package queries

import (
	"context"
	"strings"

	runnermodel "github.com/similarityyoung/simiclaw/internal/runner/model"
)

type HistoryMessage = runnermodel.HistoryMessage

func (r *Repository) LoadRecentHistory(ctx context.Context, sessionID string, limit int) ([]runnermodel.HistoryMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.Reader().QueryContext(
		ctx,
		`SELECT role, content, tool_call_id, tool_name, meta_json
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

	out := make([]runnermodel.HistoryMessage, 0, limit)
	for rows.Next() {
		var (
			msg      runnermodel.HistoryMessage
			metaJSON string
		)
		if err := rows.Scan(&msg.Role, &msg.Content, &msg.ToolCallID, &msg.ToolName, &metaJSON); err != nil {
			return nil, err
		}
		msg.Meta, msg.ToolCalls = decodeStoredMeta(metaJSON)
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	reverseHistory(out)
	return out, nil
}

func (r *Repository) LoadPromptHistory(ctx context.Context, sessionID string, limit int) ([]runnermodel.HistoryMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.Reader().QueryContext(
		ctx,
		`SELECT role, content, tool_call_id, tool_name, meta_json
		 FROM messages
		 WHERE session_id = ?
		 ORDER BY created_at DESC, fts_rowid DESC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]runnermodel.HistoryMessage, 0, limit)
	for rows.Next() {
		var (
			msg      runnermodel.HistoryMessage
			metaJSON string
		)
		if err := rows.Scan(&msg.Role, &msg.Content, &msg.ToolCallID, &msg.ToolName, &metaJSON); err != nil {
			return nil, err
		}
		msg.Meta, msg.ToolCalls = decodeStoredMeta(metaJSON)
		if shouldSkipPromptHistoryPayloadType(msg.Meta["payload_type"]) {
			continue
		}
		out = append(out, msg)
		if len(out) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	reverseHistory(out)
	return out, nil
}

func (r *Repository) SearchRAGHits(ctx context.Context, sessionID, query string, limit int) ([]runnermodel.RAGHit, error) {
	if limit <= 0 {
		limit = 5
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	rows, err := r.db.Reader().QueryContext(
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

	hits := make([]runnermodel.RAGHit, 0, limit)
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		hits = append(hits, runnermodel.RAGHit{
			Path:    sessionID,
			Scope:   "session",
			Preview: content,
		})
	}
	return hits, rows.Err()
}
