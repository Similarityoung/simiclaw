package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type Gateway interface {
	Accept(ctx context.Context, in gatewaymodel.NormalizedIngress) (gateway.AcceptedIngest, *gateway.APIError)
}

type Handler struct {
	gateway Gateway
}

func NewHandler(gateway Gateway) *Handler {
	return &Handler{gateway: gateway}
}

func (h *Handler) HandleIngest(w http.ResponseWriter, r *http.Request) {
	var req api.IngestRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteAPIError(w, &gateway.APIError{StatusCode: http.StatusBadRequest, Code: model.ErrorCodeInvalidArgument, Message: "invalid json"})
		return
	}
	normalized, apiErr := NormalizeAPIRequest(req)
	if apiErr != nil {
		WriteAPIError(w, apiErr)
		return
	}
	accepted, apiErr := h.gateway.Accept(r.Context(), normalized)
	if apiErr != nil {
		WriteAPIError(w, apiErr)
		return
	}
	WriteJSON(w, accepted.StatusCode, accepted.Response)
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteAPIError(w http.ResponseWriter, e *gateway.APIError) {
	if e == nil {
		return
	}
	if e.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(e.RetryAfter))
	}
	WriteJSON(w, e.StatusCode, api.ErrorResponse{
		Error: model.ErrorBlock{Code: e.Code, Message: e.Message, Details: e.Details},
	})
}
