package httpapi

import (
	"net/http"
	"time"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	querysvc "github.com/similarityyoung/simiclaw/internal/query"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type sessionCursor struct {
	V              int    `json:"v"`
	LastActivityAt string `json:"last_activity_at"`
	LastSessionKey string `json:"last_session_key"`
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
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
	var curTime time.Time
	hasCursor := cur.LastActivityAt != "" || cur.LastSessionKey != ""
	if hasCursor {
		t, err := time.Parse(time.RFC3339Nano, cur.LastActivityAt)
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
	page, err := s.query.ListSessions(r.Context(), querysvc.SessionListQuery{
		SessionKey:     r.URL.Query().Get("session_key"),
		ConversationID: r.URL.Query().Get("conversation_id"),
		Limit:          limit,
		Cursor: func() *querysvc.SessionCursorAnchor {
			if !hasCursor {
				return nil
			}
			return &querysvc.SessionCursorAnchor{LastActivityAt: curTime, SessionKey: cur.LastSessionKey}
		}(),
	})
	if err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: 500, Code: model.ErrorCodeInternal, Message: err.Error()})
		return
	}
	items := page.Items
	resp := map[string]any{"items": items}
	if page.Next != nil {
		resp["next_cursor"] = encodeCursor(sessionCursor{
			V:              1,
			LastActivityAt: page.Next.LastActivityAt.Format(time.RFC3339Nano),
			LastSessionKey: page.Next.SessionKey,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sessionKey := r.PathValue("session_key")
	rec, ok, err := s.db.GetSession(r.Context(), sessionKey)
	if err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: 500, Code: model.ErrorCodeInternal, Message: err.Error()})
		return
	}
	if !ok {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusNotFound,
			Code:       model.ErrorCodeNotFound,
			Message:    "session not found",
		})
		return
	}
	writeJSON(w, http.StatusOK, rec)
}
