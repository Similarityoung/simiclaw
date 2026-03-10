package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/similarityyoung/simiclaw/internal/readmodel"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (db *DB) GetSession(ctx context.Context, sessionKey string) (readmodel.SessionRecord, bool, error) {
	rows, err := db.reader.QueryContext(ctx, sessionSelectSQL+` WHERE session_key = ?`, sessionKey)
	if err != nil {
		return readmodel.SessionRecord{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return readmodel.SessionRecord{}, false, rows.Err()
	}
	rec, err := scanSession(rows)
	if err != nil {
		return readmodel.SessionRecord{}, false, err
	}
	return rec, true, rows.Err()
}

func resolveSessionTx(ctx context.Context, tx *sql.Tx, sessionKey string, conv model.Conversation, dmScope string, now time.Time) (string, error) {
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
		dmScope,
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

func scanSession(rows *sql.Rows) (readmodel.SessionRecord, error) {
	var (
		rec            readmodel.SessionRecord
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
		return readmodel.SessionRecord{}, err
	}
	rec.LastActivityAt = mustParseTime(lastActivityAt)
	rec.CreatedAt = mustParseTime(createdAt)
	rec.UpdatedAt = mustParseTime(updatedAt)
	return rec, nil
}

const sessionSelectSQL = `
SELECT session_key, active_session_id, conversation_id, channel_type, participant_id, dm_scope,
       message_count, prompt_tokens_total, completion_tokens_total, total_tokens_total,
       last_model, last_run_id, last_activity_at, created_at, updated_at
FROM sessions`
