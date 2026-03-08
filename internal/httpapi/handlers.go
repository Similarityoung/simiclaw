package httpapi

import (
	"encoding/json"
	"github.com/similarityyoung/simiclaw/internal/gateway"
	"github.com/similarityyoung/simiclaw/pkg/model"
	"net/http"
)

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	var req model.IngestRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, &gateway.APIError{StatusCode: 400, Code: model.ErrorCodeInvalidArgument, Message: "invalid json"})
		return
	}
	resp, status, apiErr := s.gateway.Ingest(r.Context(), req)
	if apiErr != nil {
		writeAPIError(w, apiErr)
		return
	}
	writeJSON(w, status, resp)
}

func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	s.handleGetEventByID(w, r, r.PathValue("event_id"))
}

func (s *Server) handleGetEventByID(w http.ResponseWriter, r *http.Request, eventID string) {
	rec, ok, err := s.db.GetEvent(r.Context(), eventID)
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
