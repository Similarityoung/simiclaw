package query

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type sessionCursor struct {
	V              int    `json:"v"`
	LastActivityAt string `json:"last_activity_at"`
	LastSessionKey string `json:"last_session_key"`
}

type messageCursor struct {
	V             int    `json:"v"`
	LastCreatedAt string `json:"last_created_at"`
	LastMessageID string `json:"last_message_id"`
}

func (h *Handlers) HandleListSessions(w http.ResponseWriter, r *http.Request) {
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
			writeAPIError(w, &gateway.APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "invalid cursor", Details: map[string]any{"field": "cursor"}})
			return
		}
		curTime = t
	}
	page, err := h.query.ListSessions(r.Context(), querymodel.SessionFilter{
		SessionKey:     r.URL.Query().Get("session_key"),
		ConversationID: r.URL.Query().Get("conversation_id"),
		Limit:          limit,
		Cursor: func() *querymodel.SessionCursorAnchor {
			if !hasCursor {
				return nil
			}
			return &querymodel.SessionCursorAnchor{LastActivityAt: curTime, SessionKey: cur.LastSessionKey}
		}(),
	})
	if err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: 500, Code: model.ErrorCodeInternal, Message: err.Error()})
		return
	}
	items := make([]api.SessionRecord, 0, len(page.Items))
	for _, rec := range page.Items {
		items = append(items, toAPISessionRecord(rec))
	}
	resp := map[string]any{"items": items}
	if page.Next != nil {
		resp["next_cursor"] = encodeCursor(sessionCursor{V: 1, LastActivityAt: page.Next.LastActivityAt.Format(time.RFC3339Nano), LastSessionKey: page.Next.SessionKey})
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (h *Handlers) HandleGetSession(w http.ResponseWriter, r *http.Request) {
	rec, ok, err := h.query.GetSession(r.Context(), r.PathValue("session_key"))
	if err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: 500, Code: model.ErrorCodeInternal, Message: err.Error()})
		return
	}
	if !ok {
		writeAPIError(w, &gateway.APIError{StatusCode: http.StatusNotFound, Code: model.ErrorCodeNotFound, Message: "session not found"})
		return
	}
	WriteJSON(w, http.StatusOK, toAPISessionRecord(rec))
}

func (h *Handlers) HandleGetSessionHistory(w http.ResponseWriter, r *http.Request) {
	sessionRec, ok, err := h.query.GetSession(r.Context(), r.PathValue("session_key"))
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
	page, err := h.query.ListSessionHistory(r.Context(), querymodel.SessionHistoryFilter{
		SessionID:   sessionRec.ActiveSessionID,
		VisibleOnly: visibleOnly,
		Limit:       limit,
		Cursor: func() *querymodel.MessageCursorAnchor {
			if before.IsZero() && cur.LastMessageID == "" {
				return nil
			}
			return &querymodel.MessageCursorAnchor{CreatedAt: before, MessageID: cur.LastMessageID}
		}(),
	})
	if err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: http.StatusInternalServerError, Code: model.ErrorCodeInternal, Message: err.Error()})
		return
	}
	items := make([]api.MessageRecord, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, toAPIMessageRecord(item))
	}
	resp := map[string]any{"items": items}
	if page.Next != nil {
		resp["next_cursor"] = encodeCursor(messageCursor{V: 1, LastCreatedAt: page.Next.CreatedAt.Format(time.RFC3339Nano), LastMessageID: page.Next.MessageID})
	}
	WriteJSON(w, http.StatusOK, resp)
}
