package queries

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (r *Repository) GetRunTrace(ctx context.Context, runID string) (querymodel.RunTrace, bool, error) {
	rows, err := r.db.Reader().QueryContext(ctx, runSelectSQL+` WHERE run_id = ?`, runID)
	if err != nil {
		return querymodel.RunTrace{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return querymodel.RunTrace{}, false, rows.Err()
	}
	trace, err := scanRun(rows)
	if err != nil {
		return querymodel.RunTrace{}, false, err
	}
	return trace, true, rows.Err()
}

func (r *Repository) ListRunTraces(ctx context.Context, filter querymodel.RunFilter) ([]querymodel.RunTrace, error) {
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
	if filter.Cursor != nil && !filter.Cursor.StartedAt.IsZero() && strings.TrimSpace(filter.Cursor.RunID) != "" {
		cursor := timeText(filter.Cursor.StartedAt)
		query += ` AND (started_at < ? OR (started_at = ? AND run_id < ?))`
		args = append(args, cursor, cursor, filter.Cursor.RunID)
	}
	query += ` ORDER BY started_at DESC, run_id DESC LIMIT ?`
	args = append(args, normalizeListLimit(filter.Limit))

	rows, err := r.db.Reader().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]querymodel.RunTrace, 0, filter.Limit)
	for rows.Next() {
		trace, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, trace)
	}
	return out, rows.Err()
}

func scanRun(rows *sql.Rows) (querymodel.RunTrace, error) {
	var (
		trace           querymodel.RunTrace
		startedAt       string
		finishedAt      string
		toolCallsJSON   string
		diagnosticsJSON string
		errorCode       string
		errorMessage    string
	)
	if err := rows.Scan(
		&trace.RunID,
		&trace.EventID,
		&trace.SessionKey,
		&trace.SessionID,
		&trace.RunMode,
		&trace.Status,
		&trace.Provider,
		&trace.Model,
		&trace.PromptTokens,
		&trace.CompletionTokens,
		&trace.TotalTokens,
		&trace.LatencyMS,
		&trace.FinishReason,
		&trace.RawFinishReason,
		&trace.ProviderRequestID,
		&trace.OutputText,
		&toolCallsJSON,
		&diagnosticsJSON,
		&startedAt,
		&finishedAt,
		&errorCode,
		&errorMessage,
	); err != nil {
		return querymodel.RunTrace{}, err
	}
	trace.StartedAt = mustParseTime(startedAt)
	trace.FinishedAt = mustParseTime(finishedAt)
	_ = json.Unmarshal([]byte(toolCallsJSON), &trace.ToolCalls)
	_ = json.Unmarshal([]byte(diagnosticsJSON), &trace.Diagnostics)
	if errorCode != "" || errorMessage != "" {
		trace.Error = &model.ErrorBlock{Code: errorCode, Message: errorMessage}
	}
	return trace, nil
}

const runSelectSQL = `
SELECT run_id, event_id, session_key, session_id, run_mode, status, provider, model,
       prompt_tokens, completion_tokens, total_tokens, latency_ms, finish_reason, raw_finish_reason,
       provider_request_id, output_text, tool_calls_json, diagnostics_json, started_at, finished_at,
       error_code, error_message
FROM runs`
