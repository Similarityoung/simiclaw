package tx

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/similarityyoung/simiclaw/internal/runtime/lanes"
	runtimemodel "github.com/similarityyoung/simiclaw/internal/runtime/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (r *RuntimeRepository) ListRunnable(ctx context.Context, limit int) ([]runtimemodel.WorkItem, error) {
	if limit <= 0 {
		limit = 64
	}
	rows, err := r.db.Reader().QueryContext(
		ctx,
		`SELECT event_id, session_key FROM events WHERE status IN (?, ?) ORDER BY updated_at ASC, event_id ASC LIMIT ?`,
		string(model.EventStatusReceived),
		string(model.EventStatusQueued),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]runtimemodel.WorkItem, 0, limit)
	for rows.Next() {
		var (
			id         string
			sessionKey string
		)
		if err := rows.Scan(&id, &sessionKey); err != nil {
			return nil, err
		}
		work := runtimemodel.WorkItem{
			EventID:    id,
			SessionKey: sessionKey,
		}
		work.LaneKey = string(lanes.Resolve(work))
		items = append(items, work)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *RuntimeRepository) ClaimWork(ctx context.Context, work runtimemodel.WorkItem, runID string, now time.Time) (runtimemodel.ClaimContext, bool, error) {
	claimed, ok, err := r.claimEvent(ctx, work.EventID, runID, now)
	if err != nil || !ok {
		return runtimemodel.ClaimContext{}, ok, err
	}
	claimedWork := work
	if claimedWork.EventID == "" {
		claimedWork.EventID = claimed.Event.EventID
	}
	claimedWork.SessionKey = claimed.SessionKey
	claimedWork.Source = claimed.Source
	channel := ""
	if claimed.Source == "telegram" {
		channel = "telegram"
	}
	claimedWork.Channel = channel
	claimedWork.LaneKey = string(lanes.Resolve(claimedWork))
	return runtimemodel.ClaimContext{
		Work:       claimedWork,
		Event:      claimed.Event,
		RunID:      claimed.RunID,
		RunMode:    claimed.RunMode,
		SessionKey: claimed.SessionKey,
		SessionID:  claimed.SessionID,
		Source:     claimedWork.Source,
		Channel:    channel,
	}, true, nil
}

func (r *RuntimeRepository) claimEvent(ctx context.Context, eventID, runID string, now time.Time) (runtimemodel.ClaimContext, bool, error) {
	var claimed runtimemodel.ClaimContext
	err := r.db.WithWriterTx(ctx, func(tx *sql.Tx) error {
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
			 RETURNING source, tenant_id, conversation_id, channel_type, participant_id, session_key, session_id, idempotency_key, payload_json, created_at`,
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
			&createdAt,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errNoMutation
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
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE events SET run_mode = ? WHERE event_id = ?`,
			string(runMode),
			eventID,
		); err != nil {
			return err
		}

		claimed = runtimemodel.ClaimContext{
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
			RunID:      runID,
			RunMode:    runMode,
			SessionKey: sessionKey,
			SessionID:  sessionID,
			Source:     source,
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errNoMutation) {
			return runtimemodel.ClaimContext{}, false, nil
		}
		return runtimemodel.ClaimContext{}, false, err
	}
	if claimed.Event.EventID == "" {
		return runtimemodel.ClaimContext{}, false, nil
	}
	return claimed, true, nil
}
