package httpapi

import (
	"net/http"
	"strings"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func (s *Server) withAuth(next http.Handler) http.Handler {
	if s.cfg.APIKey == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")) != s.cfg.APIKey {
			writeAPIError(w, &gateway.APIError{
				StatusCode: http.StatusUnauthorized,
				Code:       model.ErrorCodeUnauthorized,
				Message:    "missing or invalid api key",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
