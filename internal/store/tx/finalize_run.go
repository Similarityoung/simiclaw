package tx

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	storeprojections "github.com/similarityyoung/simiclaw/internal/store/projections"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (r *RuntimeRepository) Finalize(ctx context.Context, cmd runtimemodel.FinalizeCommand) error {
	return r.db.WithWriterTx(ctx, func(tx *sql.Tx) error {
		nowText := timeText(cmd.Now)
		for _, message := range cmd.Messages {
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

		toolCalls, err := json.Marshal(cmd.ToolCalls)
		if err != nil {
			return err
		}
		diagnostics, err := json.Marshal(cmd.Diagnostics)
		if err != nil {
			return err
		}
		errorCode := ""
		errorMessage := ""
		if cmd.Error != nil {
			errorCode = cmd.Error.Code
			errorMessage = cmd.Error.Message
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE runs
			 SET run_mode = ?, status = ?, provider = ?, model = ?, prompt_tokens = ?, completion_tokens = ?,
			     total_tokens = ?, latency_ms = ?, finish_reason = ?, raw_finish_reason = ?, provider_request_id = ?,
			     output_text = ?, tool_calls_json = ?, diagnostics_json = ?, error_code = ?, error_message = ?, finished_at = ?
			 WHERE run_id = ?`,
			string(cmd.RunMode),
			string(cmd.RunStatus),
			cmd.Provider,
			cmd.Model,
			cmd.PromptTokens,
			cmd.CompletionTokens,
			cmd.TotalTokens,
			cmd.LatencyMS,
			cmd.FinishReason,
			cmd.RawFinishReason,
			cmd.ProviderRequestID,
			cmd.OutputText,
			string(toolCalls),
			string(diagnostics),
			errorCode,
			errorMessage,
			nowText,
			cmd.RunID,
		); err != nil {
			return err
		}

		outboxID := ""
		outboxStatus := ""
		if strings.TrimSpace(cmd.OutboxBody) != "" && cmd.EventStatus == model.EventStatusProcessed {
			outboxID = fmt.Sprintf("out_%d", cmd.Now.UnixNano())
			outboxStatus = string(model.OutboxStatusPending)
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO outbox (
					outbox_id, event_id, session_key, channel, target_id, body, status, next_attempt_at, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				outboxID,
				cmd.EventID,
				cmd.SessionKey,
				cmd.OutboxChannel,
				cmd.OutboxTargetID,
				cmd.OutboxBody,
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
			string(cmd.EventStatus),
			cmd.RunID,
			string(cmd.RunMode),
			cmd.AssistantReply,
			outboxID,
			outboxStatus,
			cmd.Provider,
			cmd.Model,
			cmd.ProviderRequestID,
			errorCode,
			errorMessage,
			nowText,
			cmd.EventID,
		); err != nil {
			return err
		}
		return storeprojections.RecomputeSessionAggregateTx(ctx, tx, cmd.SessionKey, cmd.Now)
	})
}
