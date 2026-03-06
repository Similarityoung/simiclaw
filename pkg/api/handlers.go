package api

import (
	"encoding/json"
	"net/http"

	"github.com/similarityyoung/simiclaw/pkg/gateway"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (a *App) handleIngest(w http.ResponseWriter, r *http.Request) {
	var req model.IngestRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: "invalid json"})
		return
	}
	resp, status, apiErr := a.Gateway.Ingest(r.Context(), req)
	if apiErr != nil {
		writeAPIError(w, apiErr)
		return
	}
	writeJSON(w, status, resp)
}

func (a *App) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("event_id")
	rec, ok, err := a.DB.GetEvent(r.Context(), eventID)
	if err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: 500, Code: model.ErrorCodeInternal, Message: err.Error()})
		return
	}
	if !ok {
		writeAPIError(w, &gateway.APIError{StatusCode: 404, Code: model.ErrorCodeNotFound, Message: "event not found"})
		return
	}
	writeJSON(w, 200, rec)
}
