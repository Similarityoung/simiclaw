package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/gateway"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type approvalCursor struct {
	V              int    `json:"v"`
	LastCreatedAt  string `json:"last_created_at"`
	LastApprovalID string `json:"last_approval_id"`
}

func (a *App) handleCreateApproval(w http.ResponseWriter, r *http.Request) {
	var req model.CreateApprovalRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusBadRequest,
			Code:       model.ErrorCodeInvalidArgument,
			Message:    "invalid json",
		})
		return
	}
	rec, err := a.Approvals.Create(req, time.Now().UTC())
	if err != nil {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusBadRequest,
			Code:       model.ErrorCodeInvalidArgument,
			Message:    err.Error(),
		})
		return
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	base := ""
	if strings.TrimSpace(r.Host) != "" {
		base = scheme + "://" + r.Host
	}
	resp := model.CreateApprovalResponse{
		ApprovalID: rec.ApprovalID,
		Status:     rec.Status,
		ApproveURL: base + "/v1/approvals/" + rec.ApprovalID + ":approve",
		RejectURL:  base + "/v1/approvals/" + rec.ApprovalID + ":reject",
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (a *App) handleGetApproval(w http.ResponseWriter, r *http.Request) {
	approvalID := r.PathValue("approval_id")
	rec, ok, err := a.Approvals.Get(approvalID)
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
			Message:    "approval not found",
		})
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (a *App) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	limit, apiErr := parsePageLimit(r.URL.Query().Get("limit"))
	if apiErr != nil {
		writeAPIError(w, apiErr)
		return
	}
	var cur approvalCursor
	if apiErr := decodeCursor(r.URL.Query().Get("cursor"), &cur); apiErr != nil {
		writeAPIError(w, apiErr)
		return
	}
	var hasCursor bool
	var curTime time.Time
	if cur.LastCreatedAt != "" || cur.LastApprovalID != "" {
		hasCursor = true
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
		curTime = t
	}

	items, err := a.Approvals.List()
	if err != nil {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusInternalServerError,
			Code:       model.ErrorCodeInternal,
			Message:    err.Error(),
		})
		return
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ApprovalID > items[j].ApprovalID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})

	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	conversationFilter := strings.TrimSpace(r.URL.Query().Get("conversation_id"))
	sessionFilter := strings.TrimSpace(r.URL.Query().Get("session_key"))
	runFilter := strings.TrimSpace(r.URL.Query().Get("run_id"))

	filtered := make([]model.ApprovalRecord, 0, limit+1)
	for _, item := range items {
		if statusFilter != "" && string(item.Status) != statusFilter {
			continue
		}
		if conversationFilter != "" && item.ConversationID != conversationFilter {
			continue
		}
		if sessionFilter != "" && item.SessionKey != sessionFilter {
			continue
		}
		if runFilter != "" && item.RunID != runFilter {
			continue
		}
		if hasCursor {
			if item.CreatedAt.After(curTime) {
				continue
			}
			if item.CreatedAt.Equal(curTime) && item.ApprovalID >= cur.LastApprovalID {
				continue
			}
		}
		filtered = append(filtered, item)
		if len(filtered) == limit+1 {
			break
		}
	}

	resp := model.ApprovalListResponse{Items: filtered}
	if len(filtered) > limit {
		trimmed := filtered[:limit]
		last := trimmed[len(trimmed)-1]
		resp.Items = trimmed
		resp.NextCursor = encodeCursor(approvalCursor{
			V:              1,
			LastCreatedAt:  last.CreatedAt.Format(time.RFC3339Nano),
			LastApprovalID: last.ApprovalID,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) handleApprovalAction(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.PathValue("approval_action"))
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusBadRequest,
			Code:       model.ErrorCodeInvalidArgument,
			Message:    "invalid approval action path",
		})
		return
	}
	approvalID := parts[0]
	verb := parts[1]
	approve := false
	switch verb {
	case "approve":
		approve = true
	case "reject":
		approve = false
	default:
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusBadRequest,
			Code:       model.ErrorCodeInvalidArgument,
			Message:    "invalid approval action",
		})
		return
	}

	var req model.ApprovalDecisionRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusBadRequest,
			Code:       model.ErrorCodeInvalidArgument,
			Message:    "invalid json",
		})
		return
	}
	if strings.TrimSpace(req.Actor.Type) == "" || strings.TrimSpace(req.Actor.ID) == "" {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusBadRequest,
			Code:       model.ErrorCodeInvalidArgument,
			Message:    "actor.type and actor.id are required",
		})
		return
	}

	now := time.Now().UTC()
	rec, changed, conflict, notFound, err := a.Approvals.Decide(approvalID, approve, req, now)
	if err != nil {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusInternalServerError,
			Code:       model.ErrorCodeInternal,
			Message:    err.Error(),
		})
		return
	}
	if notFound {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusNotFound,
			Code:       model.ErrorCodeNotFound,
			Message:    "approval not found",
		})
		return
	}
	if conflict {
		writeAPIError(w, &gateway.APIError{
			StatusCode: http.StatusConflict,
			Code:       model.ErrorCodeConflict,
			Message:    "approval conflict",
		})
		return
	}
	if changed {
		if err := a.Approvals.PublishDecisionEvent(r.Context(), rec, now); err != nil {
			writeAPIError(w, &gateway.APIError{
				StatusCode: http.StatusInternalServerError,
				Code:       model.ErrorCodeInternal,
				Message:    err.Error(),
			})
			return
		}
	}
	writeJSON(w, http.StatusOK, model.ApprovalDecisionResponse{
		ApprovalID: rec.ApprovalID,
		Status:     rec.Status,
	})
}
