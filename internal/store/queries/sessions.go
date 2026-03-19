package queries

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
)

type MessageRecord = querymodel.MessageRecord

func (r *Repository) GetSessionRecord(ctx context.Context, sessionKey string) (querymodel.SessionRecord, bool, error) {
	rows, err := r.db.Reader().QueryContext(ctx, sessionSelectSQL+` WHERE session_key = ?`, sessionKey)
	if err != nil {
		return querymodel.SessionRecord{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return querymodel.SessionRecord{}, false, rows.Err()
	}
	rec, err := scanSession(rows)
	if err != nil {
		return querymodel.SessionRecord{}, false, err
	}
	return rec, true, rows.Err()
}

func (r *Repository) ListSessionRecords(ctx context.Context, filter querymodel.SessionFilter) ([]querymodel.SessionRecord, error) {
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
	if filter.Cursor != nil && !filter.Cursor.LastActivityAt.IsZero() && strings.TrimSpace(filter.Cursor.SessionKey) != "" {
		cursor := timeText(filter.Cursor.LastActivityAt)
		query += ` AND (last_activity_at < ? OR (last_activity_at = ? AND session_key < ?))`
		args = append(args, cursor, cursor, filter.Cursor.SessionKey)
	}
	query += ` ORDER BY last_activity_at DESC, session_key DESC LIMIT ?`
	args = append(args, normalizeListLimit(filter.Limit))

	rows, err := r.db.Reader().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]querymodel.SessionRecord, 0, filter.Limit)
	for rows.Next() {
		rec, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (r *Repository) ListMessageRecords(ctx context.Context, filter querymodel.SessionHistoryFilter) ([]querymodel.MessageRecord, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	query := `SELECT message_id, session_key, session_id, run_id, role, content, visible,
		tool_call_id, tool_name, tool_args_json, tool_result_json, meta_json, created_at
		FROM messages
		WHERE session_id = ?`
	args := []any{filter.SessionID}
	if filter.VisibleOnly {
		query += ` AND visible = 1`
	}
	if filter.Cursor != nil && (!filter.Cursor.CreatedAt.IsZero() || filter.Cursor.MessageID != "") {
		before := filter.Cursor.CreatedAt
		if before.IsZero() {
			before = time.Now().UTC().Add(24 * time.Hour)
		}
		beforeText := timeText(before)
		query += ` AND (created_at < ? OR (created_at = ? AND message_id < ?))`
		args = append(args, beforeText, beforeText, filter.Cursor.MessageID)
	}
	query += ` ORDER BY created_at DESC, message_id DESC LIMIT ?`
	args = append(args, filter.Limit)

	rows, err := r.db.Reader().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]querymodel.MessageRecord, 0, filter.Limit)
	for rows.Next() {
		rec, err := scanMessageRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	reverseMessageRecords(out)
	return out, nil
}

func scanSession(rows *sql.Rows) (querymodel.SessionRecord, error) {
	var (
		rec            querymodel.SessionRecord
		lastActivityAt string
		createdAt      string
		updatedAt      string
	)
	if err := rows.Scan(
		&rec.SessionKey,
		&rec.ActiveSessionID,
		&rec.ConversationID,
		&rec.ChannelType,
		&rec.ParticipantID,
		&rec.DMScope,
		&rec.MessageCount,
		&rec.PromptTokensTotal,
		&rec.CompletionTokensTotal,
		&rec.TotalTokensTotal,
		&rec.LastModel,
		&rec.LastRunID,
		&lastActivityAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return querymodel.SessionRecord{}, err
	}
	rec.LastActivityAt = mustParseTime(lastActivityAt)
	rec.CreatedAt = mustParseTime(createdAt)
	rec.UpdatedAt = mustParseTime(updatedAt)
	return rec, nil
}

func scanMessageRecord(rows *sql.Rows) (querymodel.MessageRecord, error) {
	var (
		rec            querymodel.MessageRecord
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
		return querymodel.MessageRecord{}, err
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
	return rec, nil
}

const sessionSelectSQL = `
SELECT session_key, active_session_id, conversation_id, channel_type, participant_id, dm_scope,
       message_count, prompt_tokens_total, completion_tokens_total, total_tokens_total,
       last_model, last_run_id, last_activity_at, created_at, updated_at
FROM sessions`
