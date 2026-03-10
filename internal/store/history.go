package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/internal/readmodel"
	"github.com/similarityyoung/simiclaw/pkg/api"
)

const historyPayloadTypeMetaKey = "payload_type"

func (db *DB) RecentMessages(ctx context.Context, sessionID string, limit int) ([]HistoryMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.reader.QueryContext(
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
	var out []HistoryMessage
	for rows.Next() {
		var msg HistoryMessage
		var metaJSON string
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

func (db *DB) RecentMessagesForPrompt(ctx context.Context, sessionID string, limit int) ([]HistoryMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.reader.QueryContext(
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

	out := make([]HistoryMessage, 0, limit)
	for rows.Next() {
		var msg HistoryMessage
		var metaJSON string
		if err := rows.Scan(&msg.Role, &msg.Content, &msg.ToolCallID, &msg.ToolName, &metaJSON); err != nil {
			return nil, err
		}
		msg.Meta, msg.ToolCalls = decodeStoredMeta(metaJSON)
		if shouldSkipPromptHistoryPayloadType(msg.Meta[historyPayloadTypeMetaKey]) {
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

func (db *DB) SearchMessagesFTS(ctx context.Context, sessionID, query string, limit int) ([]api.RAGHit, error) {
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
	var hits []api.RAGHit
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		hits = append(hits, api.RAGHit{
			Path:    sessionID,
			Scope:   "session",
			Preview: content,
		})
	}
	return hits, rows.Err()
}

func (db *DB) ListMessages(ctx context.Context, sessionID string, limit int, before time.Time, beforeMessageID string, visibleOnly bool) ([]readmodel.MessageRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT message_id, session_key, session_id, run_id, role, content, visible,
		tool_call_id, tool_name, tool_args_json, tool_result_json, meta_json, created_at
		FROM messages
		WHERE session_id = ?`
	args := []any{sessionID}
	if visibleOnly {
		query += ` AND visible = 1`
	}
	if !before.IsZero() || beforeMessageID != "" {
		if before.IsZero() {
			before = time.Now().UTC().Add(24 * time.Hour)
		}
		query += ` AND (created_at < ? OR (created_at = ? AND message_id < ?))`
		beforeText := timeText(before)
		args = append(args, beforeText, beforeText, beforeMessageID)
	}
	query += ` ORDER BY created_at DESC, message_id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]readmodel.MessageRecord, 0, limit)
	for rows.Next() {
		var (
			rec            readmodel.MessageRecord
			visible        int
			toolArgsJSON   string
			toolResultJSON string
			metaJSON       string
			createdAt      string
		)
		if err := rows.Scan(
			&rec.MessageID,
			&rec.SessionKey,
			&rec.SessionID,
			&rec.RunID,
			&rec.Role,
			&rec.Content,
			&visible,
			&rec.ToolCallID,
			&rec.ToolName,
			&toolArgsJSON,
			&toolResultJSON,
			&metaJSON,
			&createdAt,
		); err != nil {
			return nil, err
		}
		rec.Visible = visible == 1
		rec.CreatedAt = mustParseTime(createdAt)
		if strings.TrimSpace(toolArgsJSON) != "" && toolArgsJSON != "null" && toolArgsJSON != "{}" {
			_ = json.Unmarshal([]byte(toolArgsJSON), &rec.ToolArgs)
		}
		if strings.TrimSpace(toolResultJSON) != "" && toolResultJSON != "null" && toolResultJSON != "{}" {
			_ = json.Unmarshal([]byte(toolResultJSON), &rec.ToolResult)
		}
		rec.Meta, _ = decodeStoredMeta(metaJSON)
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	reverseMessages(out)
	return out, nil
}

func shouldSkipPromptHistoryPayloadType(payloadType any) bool {
	value, _ := payloadType.(string)
	return value == "cron_fire" || value == "new_session"
}

func reverseHistory(items []HistoryMessage) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func reverseMessages(items []readmodel.MessageRecord) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}
