package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	querysvc "github.com/similarityyoung/simiclaw/internal/query"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type runCursor struct {
	V             int    `json:"v"`
	LastStartedAt string `json:"last_started_at"`
	LastRunID     string `json:"last_run_id"`
}

type runSummary struct {
	RunID      string          `json:"run_id"`
	EventID    string          `json:"event_id"`
	SessionKey string          `json:"session_key"`
	SessionID  string          `json:"session_id"`
	RunMode    model.RunMode   `json:"run_mode"`
	Status     model.RunStatus `json:"status"`
	StartedAt  time.Time       `json:"started_at"`
	EndedAt    time.Time       `json:"ended_at"`
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	limit, apiErr := parsePageLimit(r.URL.Query().Get("limit"))
	if apiErr != nil {
		writeAPIError(w, apiErr)
		return
	}
	var cur runCursor
	if apiErr := decodeCursor(r.URL.Query().Get("cursor"), &cur); apiErr != nil {
		writeAPIError(w, apiErr)
		return
	}
	var curTime time.Time
	hasCursor := cur.LastStartedAt != "" || cur.LastRunID != ""
	if hasCursor {
		t, err := time.Parse(time.RFC3339Nano, cur.LastStartedAt)
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
	page, err := s.query.ListRuns(r.Context(), querysvc.RunListQuery{
		SessionKey: r.URL.Query().Get("session_key"),
		SessionID:  r.URL.Query().Get("session_id"),
		Limit:      limit,
		Cursor: func() *querysvc.RunCursorAnchor {
			if !hasCursor {
				return nil
			}
			return &querysvc.RunCursorAnchor{StartedAt: curTime, RunID: cur.LastRunID}
		}(),
	})
	if err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: 500, Code: model.ErrorCodeInternal, Message: err.Error()})
		return
	}
	items := make([]runSummary, 0, len(page.Items))
	for _, trace := range page.Items {
		items = append(items, toRunSummary(trace))
	}
	resp := map[string]any{"items": items}
	if page.Next != nil {
		resp["next_cursor"] = encodeCursor(runCursor{
			V:             1,
			LastStartedAt: page.Next.StartedAt.Format(time.RFC3339Nano),
			LastRunID:     page.Next.RunID,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	trace, ok, err := s.query.GetRun(r.Context(), runID)
	if err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: 500, Code: model.ErrorCodeInternal, Message: err.Error()})
		return
	}
	if !ok {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusNotFound,
			Code:       model.ErrorCodeNotFound,
			Message:    "run not found",
		})
		return
	}
	writeJSON(w, http.StatusOK, toRunSummary(trace))
}

func (s *Server) handleGetRunTrace(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	trace, ok, err := s.query.GetRun(r.Context(), runID)
	if err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: 500, Code: model.ErrorCodeInternal, Message: err.Error()})
		return
	}
	if !ok {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusNotFound,
			Code:       model.ErrorCodeNotFound,
			Message:    "run not found",
		})
		return
	}
	view := strings.TrimSpace(r.URL.Query().Get("view"))
	if view == "" {
		view = "full"
	}
	if view == "summary" {
		writeJSON(w, http.StatusOK, toRunSummary(trace))
		return
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("redact")); raw != "" {
		if _, err := strconv.ParseBool(raw); err != nil {
			writeAPIError(w, &gateway.APIError{
				StatusCode: http.StatusBadRequest,
				Code:       model.ErrorCodeInvalidArgument,
				Message:    "invalid redact",
				Details:    map[string]any{"field": "redact"},
			})
			return
		}
	}
	writeJSON(w, http.StatusOK, trace)
}

func toRunSummary(trace model.RunTrace) runSummary {
	return runSummary{
		RunID:      trace.RunID,
		EventID:    trace.EventID,
		SessionKey: trace.SessionKey,
		SessionID:  trace.SessionID,
		RunMode:    trace.RunMode,
		Status:     trace.Status,
		StartedAt:  trace.StartedAt,
		EndedAt:    trace.FinishedAt,
	}
}
