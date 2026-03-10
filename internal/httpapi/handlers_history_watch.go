package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	querysvc "github.com/similarityyoung/simiclaw/internal/query"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type messageCursor struct {
	V             int    `json:"v"`
	LastCreatedAt string `json:"last_created_at"`
	LastMessageID string `json:"last_message_id"`
}

func (s *Server) handleGetSessionHistory(w http.ResponseWriter, r *http.Request) {
	sessionKey := r.PathValue("session_key")
	sessionRec, ok, err := s.query.GetSession(r.Context(), sessionKey)
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

	page, err := s.query.ListSessionHistory(r.Context(), querysvc.SessionHistoryQuery{
		SessionID:   sessionRec.ActiveSessionID,
		VisibleOnly: visibleOnly,
		Limit:       limit,
		Cursor: func() *querysvc.MessageCursorAnchor {
			if before.IsZero() && cur.LastMessageID == "" {
				return nil
			}
			return &querysvc.MessageCursorAnchor{
				CreatedAt: before,
				MessageID: cur.LastMessageID,
			}
		}(),
	})
	if err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: err.Error()})
		return
	}

	apiItems := make([]api.MessageRecord, 0, len(page.Items))
	for _, item := range page.Items {
		apiItems = append(apiItems, toAPIMessageRecord(item))
	}

	resp := map[string]any{"items": apiItems}
	if page.Next != nil {
		resp["next_cursor"] = encodeCursor(messageCursor{
			V:             1,
			LastCreatedAt: page.Next.CreatedAt.Format(time.RFC3339Nano),
			LastMessageID: page.Next.MessageID,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}
