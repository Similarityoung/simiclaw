package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

var ErrIdempotencyConflict = errors.New("idempotency payload hash mismatch")

type IngestResult struct {
	EventID         string
	SessionKey      string
	SessionID       string
	ReceivedAt      time.Time
	PayloadHash     string
	Duplicate       bool
	ExistingEventID string
}

type StoredMessage struct {
	MessageID  string
	SessionKey string
	SessionID  string
	RunID      string
	Role       string
	Content    string
	Visible    bool
	ToolCallID string
	ToolName   string
	ToolArgs   map[string]any
	ToolResult map[string]any
	Meta       map[string]any
	CreatedAt  time.Time
}

type RunFinalize struct {
	RunID             string
	EventID           string
	SessionKey        string
	SessionID         string
	RunMode           model.RunMode
	RunStatus         model.RunStatus
	EventStatus       model.EventStatus
	Provider          string
	Model             string
	PromptTokens      int
	CompletionTokens  int
	TotalTokens       int
	LatencyMS         int64
	FinishReason      string
	RawFinishReason   string
	ProviderRequestID string
	OutputText        string
	ToolCalls         []model.ToolCall
	Diagnostics       map[string]string
	Error             *model.ErrorBlock
	AssistantReply    string
	Messages          []StoredMessage
	OutboxBody        string
	Now               time.Time
}

type ClaimedEvent struct {
	Event   model.InternalEvent
	RunID   string
	Status  model.EventStatus
	RunMode model.RunMode
}

type LookupEvent struct {
	EventID     string
	PayloadHash string
	ReceivedAt  time.Time
	SessionKey  string
	SessionID   string
}

type HistoryMessage struct {
	Role       string
	Content    string
	ToolCallID string
	ToolName   string
}

type ClaimedOutbox struct {
	OutboxID     string
	EventID      string
	SessionKey   string
	Body         string
	AttemptCount int
	CreatedAt    time.Time
}

func (db *DB) IngestEvent(ctx context.Context, tenantID, sessionKey string, req model.IngestRequest, payloadHash string, now time.Time) (IngestResult, error) {
	var result IngestResult
	err := db.WithWriterTx(ctx, func(tx *sql.Tx) error {
		var existing LookupEvent
		var createdAt string
		err := tx.QueryRowContext(
			ctx,
			`SELECT event_id, payload_hash, session_key, session_id, created_at FROM idempotency_keys WHERE key = ?`,
			req.IdempotencyKey,
		).Scan(&existing.EventID, &existing.PayloadHash, &existing.SessionKey, &existing.SessionID, &createdAt)
		if err == nil {
			if existing.PayloadHash != payloadHash {
				return ErrIdempotencyConflict
			}
			existing.ReceivedAt = mustParseTime(createdAt)
			result = IngestResult{
				EventID:         existing.EventID,
				SessionKey:      existing.SessionKey,
				SessionID:       existing.SessionID,
				ReceivedAt:      existing.ReceivedAt,
				PayloadHash:     existing.PayloadHash,
				Duplicate:       true,
				ExistingEventID: existing.EventID,
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		sessionID, err := resolveSessionTx(ctx, tx, sessionKey, req.Conversation, now)
		if err != nil {
			return err
		}

		eventID := fmt.Sprintf("evt_%d", now.UnixNano())
		payloadJSON, err := json.Marshal(req.Payload)
		if err != nil {
			return err
		}
		nowText := timeText(now)
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO idempotency_keys (key, event_id, payload_hash, session_key, session_id, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			req.IdempotencyKey,
			eventID,
			payloadHash,
			sessionKey,
			sessionID,
			nowText,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO events (
				event_id, source, tenant_id, conversation_id, channel_type, participant_id,
				session_key, session_id, idempotency_key, payload_type, payload_text,
				payload_json, payload_hash, status, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			eventID,
			req.Source,
			tenantID,
			req.Conversation.ConversationID,
			req.Conversation.ChannelType,
			req.Conversation.ParticipantID,
			sessionKey,
			sessionID,
			req.IdempotencyKey,
			req.Payload.Type,
			req.Payload.Text,
			string(payloadJSON),
			payloadHash,
			string(model.EventStatusReceived),
			nowText,
			nowText,
		); err != nil {
			return err
		}
		result = IngestResult{
			EventID:     eventID,
			SessionKey:  sessionKey,
			SessionID:   sessionID,
			ReceivedAt:  now,
			PayloadHash: payloadHash,
		}
		return nil
	})
	return result, err
}

func (db *DB) MarkEventQueued(ctx context.Context, eventID string, now time.Time) error {
	_, err := db.writer.ExecContext(
		ctx,
		`UPDATE events SET status = ?, updated_at = ? WHERE event_id = ? AND status = ?`,
		string(model.EventStatusQueued),
		timeText(now),
		eventID,
		string(model.EventStatusReceived),
	)
	return err
}

func (db *DB) LookupInbound(ctx context.Context, key string) (LookupEvent, bool, error) {
	var row LookupEvent
	var createdAt string
	err := db.reader.QueryRowContext(
		ctx,
		`SELECT event_id, payload_hash, session_key, session_id, created_at FROM idempotency_keys WHERE key = ?`,
		key,
	).Scan(&row.EventID, &row.PayloadHash, &row.SessionKey, &row.SessionID, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return LookupEvent{}, false, nil
	}
	if err != nil {
		return LookupEvent{}, false, err
	}
	row.ReceivedAt = mustParseTime(createdAt)
	return row, true, nil
}

func (db *DB) GetEvent(ctx context.Context, eventID string) (model.EventRecord, bool, error) {
	rows, err := db.reader.QueryContext(ctx, eventSelectSQL+` WHERE event_id = ?`, eventID)
	if err != nil {
		return model.EventRecord{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return model.EventRecord{}, false, rows.Err()
	}
	rec, err := scanEvent(rows)
	if err != nil {
		return model.EventRecord{}, false, err
	}
	return rec, true, rows.Err()
}

func (db *DB) ListEvents(ctx context.Context) ([]model.EventRecord, error) {
	rows, err := db.reader.QueryContext(ctx, eventSelectSQL+` ORDER BY created_at DESC, event_id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.EventRecord
	for rows.Next() {
		rec, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

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

func (db *DB) GetSession(ctx context.Context, sessionKey string) (model.SessionRecord, bool, error) {
	rows, err := db.reader.QueryContext(ctx, sessionSelectSQL+` WHERE session_key = ?`, sessionKey)
	if err != nil {
		return model.SessionRecord{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return model.SessionRecord{}, false, rows.Err()
	}
	rec, err := scanSession(rows)
	if err != nil {
		return model.SessionRecord{}, false, err
	}
	return rec, true, rows.Err()
}

func (db *DB) ListSessions(ctx context.Context) ([]model.SessionRecord, error) {
	rows, err := db.reader.QueryContext(ctx, sessionSelectSQL+` ORDER BY last_activity_at DESC, session_key DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.SessionRecord
	for rows.Next() {
		rec, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (db *DB) ListRunnableEventIDs(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 64
	}
	rows, err := db.reader.QueryContext(
		ctx,
		`SELECT event_id FROM events WHERE status IN (?, ?) ORDER BY updated_at ASC, event_id ASC LIMIT ?`,
		string(model.EventStatusReceived),
		string(model.EventStatusQueued),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (db *DB) ClaimEvent(ctx context.Context, eventID, runID string, now time.Time) (ClaimedEvent, bool, error) {
	var claimed ClaimedEvent
	err := db.WithWriterTx(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(
			ctx,
			`UPDATE events
			 SET status = ?, processing_started_at = ?, updated_at = ?, run_id = ?, run_mode = ?
			 WHERE event_id = ?
			   AND status IN (?, ?)
			   AND NOT EXISTS (
			       SELECT 1 FROM runs
			       WHERE runs.event_id = events.event_id AND runs.status = ?
			   )
			 RETURNING source, tenant_id, conversation_id, channel_type, participant_id, session_key, session_id, idempotency_key, payload_json, payload_type, created_at`,
			string(model.EventStatusProcessing),
			timeText(now),
			timeText(now),
			runID,
			string(model.RunModeNormal),
			eventID,
			string(model.EventStatusReceived),
			string(model.EventStatusQueued),
			string(model.RunStatusStarted),
		)

		var (
			source         string
			tenantID       string
			conversationID string
			channelType    string
			participantID  string
			sessionKey     string
			sessionID      string
			idempotencyKey string
			payloadJSON    string
			payloadType    string
			createdAt      string
		)
		if err := row.Scan(
			&source,
			&tenantID,
			&conversationID,
			&channelType,
			&participantID,
			&sessionKey,
			&sessionID,
			&idempotencyKey,
			&payloadJSON,
			&payloadType,
			&createdAt,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nilSentinel
			}
			return err
		}

		var payload model.EventPayload
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return err
		}
		runMode := model.RunModeNormal
		if isNoReplyPayload(payload.Type) {
			runMode = model.RunModeNoReply
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO runs (run_id, event_id, session_key, session_id, run_mode, status, started_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			runID,
			eventID,
			sessionKey,
			sessionID,
			string(runMode),
			string(model.RunStatusStarted),
			timeText(now),
		); err != nil {
			return err
		}
		claimed = ClaimedEvent{
			Event: model.InternalEvent{
				EventID:         eventID,
				Source:          source,
				TenantID:        tenantID,
				Conversation:    model.Conversation{ConversationID: conversationID, ChannelType: channelType, ParticipantID: participantID},
				SessionKey:      sessionKey,
				IdempotencyKey:  idempotencyKey,
				Timestamp:       mustParseTime(createdAt),
				Payload:         payload,
				ActiveSessionID: sessionID,
			},
			RunID:   runID,
			Status:  model.EventStatusProcessing,
			RunMode: runMode,
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE events SET run_mode = ? WHERE event_id = ?`,
			string(runMode),
			eventID,
		); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, nilSentinel) {
			return ClaimedEvent{}, false, nil
		}
		return ClaimedEvent{}, false, err
	}
	if claimed.Event.EventID == "" {
		return ClaimedEvent{}, false, nil
	}
	return claimed, true, nil
}

var nilSentinel = errors.New("no-op")

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
			meta, err := json.Marshal(message.Meta)
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
				string(meta),
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
					outbox_id, event_id, session_key, body, status, next_attempt_at, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				outboxID,
				finalize.EventID,
				finalize.SessionKey,
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

func (db *DB) RecoverExpiredProcessing(ctx context.Context, cutoff, now time.Time) ([]string, error) {
	rows, err := db.reader.QueryContext(
		ctx,
		`SELECT event_id FROM events
		 WHERE status = ? AND COALESCE(NULLIF(processing_started_at, ''), updated_at) <= ?
		 ORDER BY updated_at ASC, event_id ASC`,
		string(model.EventStatusProcessing),
		timeText(cutoff),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if err := db.WithWriterTx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE runs
				 SET status = ?, error_code = ?, error_message = ?, finished_at = ?
				 WHERE event_id = ? AND status = ?`,
				string(model.RunStatusFailed),
				model.ErrorCodeInternal,
				"processing lease expired",
				timeText(now),
				id,
				string(model.RunStatusStarted),
			); err != nil {
				return err
			}
			_, err := tx.ExecContext(
				ctx,
				`UPDATE events
				 SET status = ?, processing_started_at = '', updated_at = ?, error_code = ?, error_message = ?
				 WHERE event_id = ? AND status = ?`,
				string(model.EventStatusQueued),
				timeText(now),
				model.ErrorCodeInternal,
				"processing lease expired",
				id,
				string(model.EventStatusProcessing),
			)
			return err
		}); err != nil {
			return ids, err
		}
	}
	return ids, nil
}

func (db *DB) RecoverExpiredSending(ctx context.Context, cutoff, now time.Time) error {
	return db.WithWriterTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(
			ctx,
			`SELECT outbox_id, event_id FROM outbox
			 WHERE status = ? AND locked_at != '' AND locked_at <= ?`,
			string(model.OutboxStatusSending),
			timeText(cutoff),
		)
		if err != nil {
			return err
		}
		defer rows.Close()
		type row struct {
			outboxID string
			eventID  string
		}
		var expired []row
		for rows.Next() {
			var item row
			if err := rows.Scan(&item.outboxID, &item.eventID); err != nil {
				return err
			}
			expired = append(expired, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, item := range expired {
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE outbox
				 SET status = ?, locked_at = '', lock_owner = '', next_attempt_at = ?, updated_at = ?
				 WHERE outbox_id = ?`,
				string(model.OutboxStatusRetryWait),
				timeText(now),
				timeText(now),
				item.outboxID,
			); err != nil {
				return err
			}
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE events SET outbox_status = ?, updated_at = ? WHERE event_id = ?`,
				string(model.OutboxStatusRetryWait),
				timeText(now),
				item.eventID,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

func (db *DB) ClaimOutbox(ctx context.Context, owner string, now time.Time) (ClaimedOutbox, bool, error) {
	row := db.writer.QueryRowContext(
		ctx,
		`UPDATE outbox
		 SET status = ?, locked_at = ?, lock_owner = ?, updated_at = ?
		 WHERE outbox_id = (
		     SELECT outbox_id FROM outbox
		     WHERE status IN (?, ?)
		       AND next_attempt_at <= ?
		     ORDER BY next_attempt_at ASC, outbox_id ASC
		     LIMIT 1
		 )
		 RETURNING outbox_id, event_id, session_key, body, attempt_count, created_at`,
		string(model.OutboxStatusSending),
		timeText(now),
		owner,
		timeText(now),
		string(model.OutboxStatusPending),
		string(model.OutboxStatusRetryWait),
		timeText(now),
	)
	var claimed ClaimedOutbox
	var createdAt string
	err := row.Scan(&claimed.OutboxID, &claimed.EventID, &claimed.SessionKey, &claimed.Body, &claimed.AttemptCount, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ClaimedOutbox{}, false, nil
	}
	if err != nil {
		return ClaimedOutbox{}, false, err
	}
	claimed.CreatedAt = mustParseTime(createdAt)
	return claimed, true, nil
}

func (db *DB) CompleteOutboxSend(ctx context.Context, outboxID, eventID string, now time.Time) error {
	return db.WithWriterTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE outbox
			 SET status = ?, locked_at = '', lock_owner = '', sent_at = ?, updated_at = ?
			 WHERE outbox_id = ?`,
			string(model.OutboxStatusSent),
			timeText(now),
			timeText(now),
			outboxID,
		); err != nil {
			return err
		}
		_, err := tx.ExecContext(
			ctx,
			`UPDATE events SET outbox_status = ?, updated_at = ? WHERE event_id = ?`,
			string(model.OutboxStatusSent),
			timeText(now),
			eventID,
		)
		return err
	})
}

func (db *DB) FailOutboxSend(ctx context.Context, outboxID, eventID, message string, dead bool, nextAttemptAt, now time.Time) error {
	status := model.OutboxStatusRetryWait
	if dead {
		status = model.OutboxStatusDead
	}
	return db.WithWriterTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE outbox
			 SET status = ?, locked_at = '', lock_owner = '', attempt_count = attempt_count + 1,
			     last_error = ?, next_attempt_at = ?, updated_at = ?
			 WHERE outbox_id = ?`,
			string(status),
			message,
			timeText(nextAttemptAt),
			timeText(now),
			outboxID,
		); err != nil {
			return err
		}
		_, err := tx.ExecContext(
			ctx,
			`UPDATE events SET outbox_status = ?, updated_at = ? WHERE event_id = ?`,
			string(status),
			timeText(now),
			eventID,
		)
		return err
	})
}

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

func resolveSessionTx(ctx context.Context, tx *sql.Tx, sessionKey string, conv model.Conversation, now time.Time) (string, error) {
	var sessionID string
	err := tx.QueryRowContext(ctx, `SELECT active_session_id FROM sessions WHERE session_key = ?`, sessionKey).Scan(&sessionID)
	if err == nil {
		return sessionID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	sessionID = fmt.Sprintf("ses_%d", now.UnixNano())
	nowText := timeText(now)
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO sessions (
			session_key, active_session_id, conversation_id, channel_type, participant_id, dm_scope,
			last_activity_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionKey,
		sessionID,
		conv.ConversationID,
		conv.ChannelType,
		conv.ParticipantID,
		"default",
		nowText,
		nowText,
		nowText,
	)
	return sessionID, err
}

func recomputeSessionAggregateTx(ctx context.Context, tx *sql.Tx, sessionKey string, now time.Time) error {
	var (
		messageCount    int
		promptTotal     int
		completionTotal int
		totalTokens     int
		lastModel       sql.NullString
		lastRunID       sql.NullString
		lastActivity    sql.NullString
	)
	if err := tx.QueryRowContext(
		ctx,
		`SELECT COUNT(1)
		 FROM messages
		 WHERE session_key = ? AND visible = 1 AND role IN ('user', 'assistant', 'tool')`,
		sessionKey,
	).Scan(&messageCount); err != nil {
		return err
	}
	if err := tx.QueryRowContext(
		ctx,
		`SELECT COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(total_tokens), 0)
		 FROM runs
		 WHERE session_key = ? AND status = ?`,
		sessionKey,
		string(model.RunStatusCompleted),
	).Scan(&promptTotal, &completionTotal, &totalTokens); err != nil {
		return err
	}
	if err := tx.QueryRowContext(
		ctx,
		`SELECT model, run_id, finished_at
		 FROM runs
		 WHERE session_key = ? AND status = ?
		 ORDER BY finished_at DESC, run_id DESC
		 LIMIT 1`,
		sessionKey,
		string(model.RunStatusCompleted),
	).Scan(&lastModel, &lastRunID, &lastActivity); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	lastActivityAt := timeText(now)
	if lastActivity.Valid {
		lastActivityAt = lastActivity.String
	}
	_, err := tx.ExecContext(
		ctx,
		`UPDATE sessions
		 SET message_count = ?, prompt_tokens_total = ?, completion_tokens_total = ?, total_tokens_total = ?,
		     last_model = ?, last_run_id = ?, last_activity_at = ?, updated_at = ?
		 WHERE session_key = ?`,
		messageCount,
		promptTotal,
		completionTotal,
		totalTokens,
		lastModel.String,
		lastRunID.String,
		lastActivityAt,
		timeText(now),
		sessionKey,
	)
	return err
}

func scanEvent(rows *sql.Rows) (model.EventRecord, error) {
	var (
		rec          model.EventRecord
		createdAt    string
		receivedAt   string
		updatedAt    string
		errorCode    string
		errorMessage string
	)
	if err := rows.Scan(
		&rec.EventID,
		&rec.Status,
		&rec.OutboxStatus,
		&rec.SessionKey,
		&rec.SessionID,
		&rec.RunID,
		&rec.RunMode,
		&rec.AssistantReply,
		&rec.OutboxID,
		&rec.ProcessingLease,
		&rec.PayloadHash,
		&rec.Provider,
		&rec.Model,
		&rec.ProviderRequestID,
		&createdAt,
		&receivedAt,
		&updatedAt,
		&errorCode,
		&errorMessage,
	); err != nil {
		return model.EventRecord{}, err
	}
	rec.CreatedAt = mustParseTime(createdAt)
	rec.ReceivedAt = mustParseTime(receivedAt)
	rec.UpdatedAt = mustParseTime(updatedAt)
	if errorCode != "" || errorMessage != "" {
		rec.Error = &model.ErrorBlock{Code: errorCode, Message: errorMessage}
	}
	return rec, nil
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

func scanSession(rows *sql.Rows) (model.SessionRecord, error) {
	var (
		rec            model.SessionRecord
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
		return model.SessionRecord{}, err
	}
	rec.LastActivityAt = mustParseTime(lastActivityAt)
	rec.CreatedAt = mustParseTime(createdAt)
	rec.UpdatedAt = mustParseTime(updatedAt)
	return rec, nil
}

func reverseHistory(items []HistoryMessage) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func timeText(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func mustParseTime(raw string) time.Time {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func isNoReplyPayload(payloadType string) bool {
	return payloadType == "memory_flush" || payloadType == "compaction" || payloadType == "cron_fire"
}

const eventSelectSQL = `
SELECT event_id, status, outbox_status, session_key, session_id, run_id, run_mode, assistant_reply,
       outbox_id, processing_started_at, payload_hash, provider, model, provider_request_id,
       created_at, created_at, updated_at, error_code, error_message
FROM events`

const runSelectSQL = `
SELECT run_id, event_id, session_key, session_id, run_mode, status, provider, model,
       prompt_tokens, completion_tokens, total_tokens, latency_ms, finish_reason, raw_finish_reason,
       provider_request_id, output_text, tool_calls_json, diagnostics_json, started_at, finished_at,
       error_code, error_message
FROM runs`

const sessionSelectSQL = `
SELECT session_key, active_session_id, conversation_id, channel_type, participant_id, dm_scope,
       message_count, prompt_tokens_total, completion_tokens_total, total_tokens_total,
       last_model, last_run_id, last_activity_at, created_at, updated_at
FROM sessions`
