package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeAPIError(w http.ResponseWriter, e *gateway.APIError) {
	if e.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(e.RetryAfter))
	}
	writeJSON(w, e.StatusCode, model.ErrorResponse{Error: model.ErrorBlock{Code: e.Code, Message: e.Message, Details: e.Details}})
}
