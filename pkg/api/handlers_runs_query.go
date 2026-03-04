package api

import (
	"encoding/json"
	"net/http"
	"sort"
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
	RunID      string        `json:"run_id"`
	EventID    string        `json:"event_id"`
	SessionKey string        `json:"session_key"`
	SessionID  string        `json:"session_id"`
	RunMode    model.RunMode `json:"run_mode"`
	Status     string        `json:"status"`
	StartedAt  time.Time     `json:"started_at"`
	EndedAt    time.Time     `json:"ended_at"`
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
	var hasCursor bool
	var curTime time.Time
	if cur.LastStartedAt != "" || cur.LastRunID != "" {
		hasCursor = true
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

	traces, err := a.Runs.List()
	if err != nil {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusInternalServerError,
			Code:       model.ErrorCodeInternal,
			Message:    err.Error(),
		})
		return
	}

	sort.Slice(traces, func(i, j int) bool {
		if traces[i].StartedAt.Equal(traces[j].StartedAt) {
			return traces[i].RunID > traces[j].RunID
		}
		return traces[i].StartedAt.After(traces[j].StartedAt)
	})

	conversationID := r.URL.Query().Get("conversation_id")
	sessionKey := r.URL.Query().Get("session_key")
	sessionID := r.URL.Query().Get("session_id")
	sessions := a.Sessions.Snapshot().Sessions

	items := make([]runSummary, 0, limit+1)
	for _, trace := range traces {
		if sessionKey != "" && trace.SessionKey != sessionKey {
			continue
		}
		if sessionID != "" && trace.SessionID != sessionID {
			continue
		}
		if conversationID != "" {
			row, ok := sessions[trace.SessionKey]
			if !ok || row.ConversationID != conversationID {
				continue
			}
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
	trace, ok, err := a.Runs.Get(runID)
	if err != nil {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusInternalServerError,
			Code:       model.ErrorCodeInternal,
			Message:    err.Error(),
		})
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
	trace, ok, err := a.Runs.Get(runID)
	if err != nil {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusInternalServerError,
			Code:       model.ErrorCodeInternal,
			Message:    err.Error(),
		})
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
	if view != "full" && view != "summary" {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusBadRequest,
			Code:       model.ErrorCodeInvalidArgument,
			Message:    "invalid view",
			Details:    map[string]any{"field": "view"},
		})
		return
	}
	redact := false
	if raw := strings.TrimSpace(r.URL.Query().Get("redact")); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			writeAPIError(w, &gateway.APIError{
				StatusCode: http.StatusBadRequest,
				Code:       model.ErrorCodeInvalidArgument,
				Message:    "invalid redact",
				Details:    map[string]any{"field": "redact"},
			})
			return
		}
		redact = v
	}

	if view == "summary" {
		writeJSON(w, http.StatusOK, toRunSummary(trace))
		return
	}
	if !redact {
		writeJSON(w, http.StatusOK, trace)
		return
	}
	writeJSON(w, http.StatusOK, redactTrace(trace))
}

func toRunSummary(trace model.RunTrace) runSummary {
	status := "succeeded"
	if trace.Error != nil {
		status = "failed"
	}
	return runSummary{
		RunID:      trace.RunID,
		EventID:    trace.EventID,
		SessionKey: trace.SessionKey,
		SessionID:  trace.SessionID,
		RunMode:    trace.RunMode,
		Status:     status,
		StartedAt:  trace.StartedAt,
		EndedAt:    trace.FinishedAt,
	}
}

func redactTrace(trace model.RunTrace) map[string]any {
	b, _ := json.Marshal(trace)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return redactMap(m)
}

func redactMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		lk := strings.ToLower(k)
		switch lk {
		case "diff":
			out[k] = "<redacted>"
		case "content":
			out[k] = "<redacted>"
		case "native", "native_ref":
			out[k] = "<redacted>"
		case "args":
			out[k] = "<redacted>"
		default:
			out[k] = redactValue(v)
		}
	}
	return out
}

func redactValue(v any) any {
	switch tv := v.(type) {
	case map[string]any:
		return redactMap(tv)
	case []any:
		out := make([]any, 0, len(tv))
		for _, item := range tv {
			out = append(out, redactValue(item))
		}
		return out
	default:
		return v
	}
}
