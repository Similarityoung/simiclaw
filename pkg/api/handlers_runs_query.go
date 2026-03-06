package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/gateway"
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

func (a *App) handleListRuns(w http.ResponseWriter, r *http.Request) {
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
	runs, err := a.DB.ListRuns(r.Context())
	if err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: 500, Code: model.ErrorCodeInternal, Message: err.Error()})
		return
	}
	sessionKey := r.URL.Query().Get("session_key")
	sessionID := r.URL.Query().Get("session_id")
	items := make([]runSummary, 0, limit+1)
	for _, trace := range runs {
		if sessionKey != "" && trace.SessionKey != sessionKey {
			continue
		}
		if sessionID != "" && trace.SessionID != sessionID {
			continue
		}
		if hasCursor {
			if trace.StartedAt.After(curTime) {
				continue
			}
			if trace.StartedAt.Equal(curTime) && trace.RunID >= cur.LastRunID {
				continue
			}
		}
		items = append(items, toRunSummary(trace))
		if len(items) == limit+1 {
			break
		}
	}
	resp := map[string]any{"items": items}
	if len(items) > limit {
		trimmed := items[:limit]
		last := trimmed[len(trimmed)-1]
		resp["items"] = trimmed
		resp["next_cursor"] = encodeCursor(runCursor{
			V:             1,
			LastStartedAt: last.StartedAt.Format(time.RFC3339Nano),
			LastRunID:     last.RunID,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) handleGetRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	trace, ok, err := a.DB.GetRun(r.Context(), runID)
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

func (a *App) handleGetRunTrace(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	trace, ok, err := a.DB.GetRun(r.Context(), runID)
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
