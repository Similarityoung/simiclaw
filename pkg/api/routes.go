package api

import "net/http"

// routes 注册健康检查、就绪检查和业务 API 路由。
func (a *App) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", a.handleHealthz)
	mux.HandleFunc("GET /readyz", a.handleReadyz)

	mux.Handle("POST /v1/events:ingest", a.withAuth(http.HandlerFunc(a.handleIngest)))
	mux.Handle("GET /v1/events/{event_id}", a.withAuth(http.HandlerFunc(a.handleGetEvent)))

	return mux
}
