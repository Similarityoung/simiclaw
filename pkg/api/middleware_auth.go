package api

import (
	"net/http"
	"strings"

	"github.com/similarityyoung/simiclaw/pkg/gateway"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

// withAuth 在配置了 APIKey 时校验 Bearer Token，否则直接放行请求。
func (a *App) withAuth(next http.Handler) http.Handler {
	if a.Cfg.APIKey == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")) != a.Cfg.APIKey {
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
