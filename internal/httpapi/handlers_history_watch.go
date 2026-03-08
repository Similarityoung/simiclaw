package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type messageCursor struct {
	V             int    `json:"v"`
	LastCreatedAt string `json:"last_created_at"`
	LastMessageID string `json:"last_message_id"`
}

func (s *Server) handleGetSessionHistory(w http.ResponseWriter, r *http.Request) {
	sessionKey := r.PathValue("session_key")
	sessionRec, ok, err := s.db.GetSession(r.Context(), sessionKey)
	if err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: err.Error()})
		return
	}
	if !ok {
		writeAPIError(w, &gateway.APIError{StatusCode: http.StatusNotFound, Code: model.ErrorCodeNotFound, Message: "session not found"})
		return
	}

	limit, apiErr := parsePageLimit(r.URL.Query().Get("limit"))
	if apiErr != nil {
		writeAPIError(w, apiErr)
		return
	}

	var cur messageCursor
	if apiErr := decodeCursor(r.URL.Query().Get("cursor"), &cur); apiErr != nil {
		writeAPIError(w, apiErr)
		return
	}

	var before time.Time
	if cur.LastCreatedAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, cur.LastCreatedAt)
		if err != nil {
			writeAPIError(w, &gateway.APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "invalid cursor", Details: map[string]any{"field": "cursor"}})
			return
		}
		before = parsed
	}

	visibleOnly := true
	if raw := strings.TrimSpace(r.URL.Query().Get("visible")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			writeAPIError(w, &gateway.APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "invalid visible", Details: map[string]any{"field": "visible"}})
			return
		}
		visibleOnly = parsed
	}

	items, err := s.db.ListMessages(r.Context(), sessionRec.ActiveSessionID, limit+1, before, cur.LastMessageID, visibleOnly)
	if err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: err.Error()})
		return
	}

	resp := map[string]any{"items": items}
	if len(items) > limit {
		trimmed := items[1:]
		resp["items"] = trimmed
		first := trimmed[0]
		resp["next_cursor"] = encodeCursor(messageCursor{
			V:             1,
			LastCreatedAt: first.CreatedAt.Format(time.RFC3339Nano),
			LastMessageID: first.MessageID,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}
