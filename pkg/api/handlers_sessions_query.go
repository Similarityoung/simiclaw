package api

import (
	"net/http"
	"sort"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/gateway"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type sessionCursor struct {
	V              int    `json:"v"`
	LastUpdatedAt  string `json:"last_updated_at"`
	LastSessionKey string `json:"last_session_key"`
}

type sessionListItem struct {
	SessionKey      string    `json:"session_key"`
	ActiveSessionID string    `json:"active_session_id"`
	ConversationID  string    `json:"conversation_id,omitempty"`
	ChannelType     string    `json:"channel_type,omitempty"`
	ParticipantID   string    `json:"participant_id,omitempty"`
	DMScope         string    `json:"dm_scope,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
	LastCommitID    string    `json:"last_commit_id,omitempty"`
	LastRunID       string    `json:"last_run_id,omitempty"`
}

func (a *App) handleListSessions(w http.ResponseWriter, r *http.Request) {
	limit, apiErr := parsePageLimit(r.URL.Query().Get("limit"))
	if apiErr != nil {
		writeAPIError(w, apiErr)
		return
	}

	var cur sessionCursor
	if apiErr := decodeCursor(r.URL.Query().Get("cursor"), &cur); apiErr != nil {
		writeAPIError(w, apiErr)
		return
	}
	var hasCursor bool
	var curTime time.Time
	if cur.LastUpdatedAt != "" || cur.LastSessionKey != "" {
		hasCursor = true
		t, err := time.Parse(time.RFC3339Nano, cur.LastUpdatedAt)
		if err != nil {
			writeAPIError(w, &gateway.APIError{
				StatusCode: http.StatusBadRequest,
				Code:       model.ErrorCodeInvalidArgument,
				Message:    "invalid cursor",
				Details:    map[string]any{"field": "cursor"},
			})
			return
		}
		curTime = t
	}

	sessionKeyFilter := r.URL.Query().Get("session_key")
	conversationID := r.URL.Query().Get("conversation_id")
	snap := a.Sessions.Snapshot().Sessions

	items := make([]sessionListItem, 0, len(snap))
	for sessionKey, row := range snap {
		if sessionKeyFilter != "" && sessionKey != sessionKeyFilter {
			continue
		}
		if conversationID != "" && row.ConversationID != conversationID {
			continue
		}
		items = append(items, sessionListItem{
			SessionKey:      sessionKey,
			ActiveSessionID: row.ActiveSessionID,
			ConversationID:  row.ConversationID,
			ChannelType:     row.ChannelType,
			ParticipantID:   row.ParticipantID,
			DMScope:         row.DMScope,
			UpdatedAt:       row.UpdatedAt,
			LastCommitID:    row.LastCommitID,
			LastRunID:       row.LastRunID,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].SessionKey < items[j].SessionKey
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})

	filtered := make([]sessionListItem, 0, limit+1)
	for _, item := range items {
		if hasCursor {
			if item.UpdatedAt.After(curTime) {
				continue
			}
			if item.UpdatedAt.Equal(curTime) && item.SessionKey <= cur.LastSessionKey {
				continue
			}
		}
		filtered = append(filtered, item)
		if len(filtered) == limit+1 {
			break
		}
	}

	resp := map[string]any{"items": filtered}
	if len(filtered) > limit {
		trimmed := filtered[:limit]
		last := trimmed[len(trimmed)-1]
		resp["items"] = trimmed
		resp["next_cursor"] = encodeCursor(sessionCursor{
			V:              1,
			LastUpdatedAt:  last.UpdatedAt.Format(time.RFC3339Nano),
			LastSessionKey: last.SessionKey,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sessionKey := r.PathValue("session_key")
	row, ok := a.Sessions.Get(sessionKey)
	if !ok {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusNotFound,
			Code:       model.ErrorCodeNotFound,
			Message:    "session not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, sessionListItem{
		SessionKey:      sessionKey,
		ActiveSessionID: row.ActiveSessionID,
		ConversationID:  row.ConversationID,
		ChannelType:     row.ChannelType,
		ParticipantID:   row.ParticipantID,
		DMScope:         row.DMScope,
		UpdatedAt:       row.UpdatedAt,
		LastCommitID:    row.LastCommitID,
		LastRunID:       row.LastRunID,
	})
}
