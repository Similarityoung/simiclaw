package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (db *DB) GetRun(ctx context.Context, runID string) (model.RunTrace, bool, error) {
	rows, err := db.reader.QueryContext(ctx, runSelectSQL+` WHERE run_id = ?`, runID)
	if err != nil {
		return model.RunTrace{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return model.RunTrace{}, false, rows.Err()
	}
	trace, err := scanRun(rows)
	if err != nil {
		return model.RunTrace{}, false, err
	}
	return trace, true, rows.Err()
}

func (db *DB) ListRuns(ctx context.Context) ([]model.RunTrace, error) {
	rows, err := db.reader.QueryContext(ctx, runSelectSQL+` ORDER BY started_at DESC, run_id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.RunTrace
	for rows.Next() {
		trace, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, trace)
	}
	return out, rows.Err()
}

func (db *DB) FinalizeRun(ctx context.Context, finalize RunFinalize) error {
	return db.WithWriterTx(ctx, func(tx *sql.Tx) error {
		nowText := timeText(finalize.Now)
		for _, message := range finalize.Messages {
			toolArgs, err := json.Marshal(message.ToolArgs)
			if err != nil {
				return err
			}
			toolResult, err := json.Marshal(message.ToolResult)
			if err != nil {
				return err
			}
			meta, err := encodeStoredMeta(message.Meta, message.ToolCalls)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO messages (
					message_id, session_key, session_id, run_id, role, content, visible,
					tool_call_id, tool_name, tool_args_json, tool_result_json, meta_json, created_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				message.MessageID,
				message.SessionKey,
				message.SessionID,
				message.RunID,
				message.Role,
				message.Content,
				boolToInt(message.Visible),
				message.ToolCallID,
				message.ToolName,
				string(toolArgs),
				string(toolResult),
				meta,
				timeText(message.CreatedAt),
			); err != nil {
				return err
			}
		}

		toolCalls, err := json.Marshal(finalize.ToolCalls)
		if err != nil {
			return err
		}
		diagnostics, err := json.Marshal(finalize.Diagnostics)
		if err != nil {
			return err
		}
		errorCode := ""
		errorMessage := ""
		if finalize.Error != nil {
			errorCode = finalize.Error.Code
			errorMessage = finalize.Error.Message
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE runs
			 SET run_mode = ?, status = ?, provider = ?, model = ?, prompt_tokens = ?, completion_tokens = ?,
			     total_tokens = ?, latency_ms = ?, finish_reason = ?, raw_finish_reason = ?, provider_request_id = ?,
			     output_text = ?, tool_calls_json = ?, diagnostics_json = ?, error_code = ?, error_message = ?, finished_at = ?
			 WHERE run_id = ?`,
			string(finalize.RunMode),
			string(finalize.RunStatus),
			finalize.Provider,
			finalize.Model,
			finalize.PromptTokens,
			finalize.CompletionTokens,
			finalize.TotalTokens,
			finalize.LatencyMS,
			finalize.FinishReason,
			finalize.RawFinishReason,
			finalize.ProviderRequestID,
			finalize.OutputText,
			string(toolCalls),
			string(diagnostics),
			errorCode,
			errorMessage,
			nowText,
			finalize.RunID,
		); err != nil {
			return err
		}

		outboxID := ""
		outboxStatus := ""
		if strings.TrimSpace(finalize.OutboxBody) != "" && finalize.EventStatus == model.EventStatusProcessed {
			outboxID = fmt.Sprintf("out_%d", finalize.Now.UnixNano())
			outboxStatus = string(model.OutboxStatusPending)
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO outbox (
					outbox_id, event_id, session_key, channel, target_id, body, status, next_attempt_at, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				outboxID,
				finalize.EventID,
				finalize.SessionKey,
				finalize.OutboxChannel,
				finalize.OutboxTargetID,
				finalize.OutboxBody,
				outboxStatus,
				nowText,
				nowText,
				nowText,
			); err != nil {
				return err
			}
		}

		if _, err := tx.ExecContext(
			ctx,
			`UPDATE events
			 SET status = ?, run_id = ?, run_mode = ?, assistant_reply = ?, outbox_id = ?, outbox_status = ?,
			     processing_started_at = '', provider = ?, model = ?, provider_request_id = ?, error_code = ?, error_message = ?, updated_at = ?
			 WHERE event_id = ?`,
			string(finalize.EventStatus),
			finalize.RunID,
			string(finalize.RunMode),
			finalize.AssistantReply,
			outboxID,
			outboxStatus,
			finalize.Provider,
			finalize.Model,
			finalize.ProviderRequestID,
			errorCode,
			errorMessage,
			nowText,
			finalize.EventID,
		); err != nil {
			return err
		}
		return recomputeSessionAggregateTx(ctx, tx, finalize.SessionKey, finalize.Now)
	})
}

func scanRun(rows *sql.Rows) (model.RunTrace, error) {
	var (
		trace           model.RunTrace
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
		return model.RunTrace{}, err
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
