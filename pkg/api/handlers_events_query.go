package api

import (
	"net/http"
	"sort"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/gateway"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type eventCursor struct {
	V             int    `json:"v"`
	LastUpdatedAt string `json:"last_updated_at"`
	LastEventID   string `json:"last_event_id"`
}

type eventListItem struct {
	EventID        string               `json:"event_id"`
	Status         model.EventStatus    `json:"status"`
	DeliveryStatus model.DeliveryStatus `json:"delivery_status"`
	SessionKey     string               `json:"session_key"`
	SessionID      string               `json:"session_id"`
	RunID          string               `json:"run_id,omitempty"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

func (a *App) handleListEvents(w http.ResponseWriter, r *http.Request) {
	limit, apiErr := parsePageLimit(r.URL.Query().Get("limit"))
	if apiErr != nil {
		writeAPIError(w, apiErr)
		return
	}

	var cur eventCursor
	if apiErr := decodeCursor(r.URL.Query().Get("cursor"), &cur); apiErr != nil {
		writeAPIError(w, apiErr)
		return
	}
	var hasCursor bool
	var curTime time.Time
	if cur.LastUpdatedAt != "" || cur.LastEventID != "" {
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

	conversationID := r.URL.Query().Get("conversation_id")
	sessionKey := r.URL.Query().Get("session_key")
	statusFilter := model.EventStatus(r.URL.Query().Get("status"))
	deliveryFilter := model.DeliveryStatus(r.URL.Query().Get("delivery_status"))

	events := a.Events.List()
	sort.Slice(events, func(i, j int) bool {
		if events[i].UpdatedAt.Equal(events[j].UpdatedAt) {
			return events[i].EventID > events[j].EventID
		}
		return events[i].UpdatedAt.After(events[j].UpdatedAt)
	})

	sessions := a.Sessions.Snapshot().Sessions
	items := make([]eventListItem, 0, limit+1)
	for _, rec := range events {
		if sessionKey != "" && rec.SessionKey != sessionKey {
			continue
		}
		if statusFilter != "" && rec.Status != statusFilter {
			continue
		}
		if deliveryFilter != "" && rec.DeliveryStatus != deliveryFilter {
			continue
		}
		if conversationID != "" {
			row, ok := sessions[rec.SessionKey]
			if !ok || row.ConversationID != conversationID {
				continue
			}
		}
		if hasCursor {
			if rec.UpdatedAt.After(curTime) {
				continue
			}
			if rec.UpdatedAt.Equal(curTime) && rec.EventID >= cur.LastEventID {
				continue
			}
		}
		items = append(items, eventListItem{
			EventID:        rec.EventID,
			Status:         rec.Status,
			DeliveryStatus: rec.DeliveryStatus,
			SessionKey:     rec.SessionKey,
			SessionID:      rec.SessionID,
			RunID:          rec.RunID,
			UpdatedAt:      rec.UpdatedAt,
		})
		if len(items) == limit+1 {
			break
		}
	}

	resp := map[string]any{"items": items}
	if len(items) > limit {
		trimmed := items[:limit]
		last := trimmed[len(trimmed)-1]
		resp["items"] = trimmed
		resp["next_cursor"] = encodeCursor(eventCursor{
			V:             1,
			LastUpdatedAt: last.UpdatedAt.Format(time.RFC3339Nano),
			LastEventID:   last.EventID,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) handleLookupEvent(w http.ResponseWriter, r *http.Request) {
	idempotencyKey := r.URL.Query().Get("idempotency_key")
	if idempotencyKey == "" {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusBadRequest,
			Code:       model.ErrorCodeInvalidArgument,
			Message:    "idempotency_key is required",
			Details:    map[string]any{"field": "idempotency_key"},
		})
		return
	}

	row, ok := a.Idempotency.LookupInbound(idempotencyKey)
	if !ok {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusNotFound,
			Code:       model.ErrorCodeNotFound,
			Message:    "idempotency key not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"event_id":     row.EventID,
		"payload_hash": row.PayloadHash,
		"received_at":  row.ReceivedAt,
		"status_url":   "/v1/events/" + row.EventID,
	})
}
