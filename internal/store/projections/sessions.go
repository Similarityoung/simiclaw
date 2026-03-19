package projections

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func ResolveSessionTx(ctx context.Context, tx *sql.Tx, sessionKey string, conv model.Conversation, dmScope string, now time.Time) (string, error) {
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

func RecomputeSessionAggregateTx(ctx context.Context, tx *sql.Tx, sessionKey string, now time.Time) error {
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

func timeText(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
