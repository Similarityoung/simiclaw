package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func WithAPIKey(apiKey string, next http.Handler) http.Handler {
	if apiKey == "" {
		return next
	}
	logger := logging.L("http.auth")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")) != apiKey {
			logger.Warn("api key rejected",
				logging.String("method", r.Method),
				logging.String("path", r.URL.Path),
				logging.String("error_code", model.ErrorCodeUnauthorized),
				logging.Int("status_code", http.StatusUnauthorized),
			)
			writeAPIError(w, http.StatusUnauthorized, model.ErrorCodeUnauthorized, "missing or invalid api key", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeAPIError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{
		Error: model.ErrorBlock{Code: code, Message: message, Details: details},
	})
}
