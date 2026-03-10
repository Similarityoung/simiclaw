package httpapi

import (
	"net/http"
	"time"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type eventCursor struct {
	V             int    `json:"v"`
	LastCreatedAt string `json:"last_created_at"`
	LastEventID   string `json:"last_event_id"`
}

type eventListItem struct {
	EventID      string             `json:"event_id"`
	Status       model.EventStatus  `json:"status"`
	OutboxStatus model.OutboxStatus `json:"outbox_status,omitempty"`
	SessionKey   string             `json:"session_key"`
	SessionID    string             `json:"session_id"`
	RunID        string             `json:"run_id,omitempty"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
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
	var cursorTime time.Time
	hasCursor := cur.LastCreatedAt != "" || cur.LastEventID != ""
	if hasCursor {
		t, err := time.Parse(time.RFC3339Nano, cur.LastCreatedAt)
		if err != nil {
			writeAPIError(w, &gateway.APIError{
				StatusCode: http.StatusBadRequest,
				Code:       model.ErrorCodeInvalidArgument,
				Message:    "invalid cursor",
				Details:    map[string]any{"field": "cursor"},
			})
			return
		}
		cursorTime = t
	}

	page, err := s.query.ListEvents(r.Context(), querymodel.EventFilter{
		SessionKey: r.URL.Query().Get("session_key"),
		Status:     model.EventStatus(r.URL.Query().Get("status")),
		Limit:      limit,
		Cursor: func() *querymodel.EventCursorAnchor {
			if !hasCursor {
				return nil
			}
			return &querymodel.EventCursorAnchor{CreatedAt: cursorTime, EventID: cur.LastEventID}
		}(),
	})
	if err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: 500, Code: model.ErrorCodeInternal, Message: err.Error()})
		return
	}
	items := make([]eventListItem, 0, len(page.Items))
	for _, rec := range page.Items {
		items = append(items, eventListItem{
			EventID:      rec.EventID,
			Status:       rec.Status,
			OutboxStatus: rec.OutboxStatus,
			SessionKey:   rec.SessionKey,
			SessionID:    rec.SessionID,
			RunID:        rec.RunID,
			UpdatedAt:    rec.UpdatedAt,
		})
	}

	resp := map[string]any{"items": items}
	if page.Next != nil {
		resp["next_cursor"] = encodeCursor(eventCursor{
			V:             1,
			LastCreatedAt: page.Next.CreatedAt.Format(time.RFC3339Nano),
			LastEventID:   page.Next.EventID,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleLookupEvent(w http.ResponseWriter, r *http.Request) {
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
	row, ok, err := s.query.LookupEvent(r.Context(), idempotencyKey)
	if err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: 500, Code: model.ErrorCodeInternal, Message: err.Error()})
		return
	}
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
