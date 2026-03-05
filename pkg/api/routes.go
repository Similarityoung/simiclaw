package api

import "net/http"

// routes 注册健康检查、就绪检查和业务 API 路由。
func (a *App) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", a.handleHealthz)
	mux.HandleFunc("GET /readyz", a.handleReadyz)

	mux.Handle("POST /v1/events:ingest", a.withAuth(http.HandlerFunc(a.handleIngest)))
	mux.Handle("GET /v1/events/{event_id}", a.withAuth(http.HandlerFunc(a.handleGetEvent)))
	mux.Handle("GET /v1/events", a.withAuth(http.HandlerFunc(a.handleListEvents)))
	mux.Handle("GET /v1/events:lookup", a.withAuth(http.HandlerFunc(a.handleLookupEvent)))
	mux.Handle("GET /v1/runs", a.withAuth(http.HandlerFunc(a.handleListRuns)))
	mux.Handle("GET /v1/runs/{run_id}", a.withAuth(http.HandlerFunc(a.handleGetRun)))
	mux.Handle("GET /v1/runs/{run_id}/trace", a.withAuth(http.HandlerFunc(a.handleGetRunTrace)))
	mux.Handle("GET /v1/sessions", a.withAuth(http.HandlerFunc(a.handleListSessions)))
	mux.Handle("GET /v1/sessions/{session_key}", a.withAuth(http.HandlerFunc(a.handleGetSession)))
	mux.Handle("POST /v1/approvals", a.withAuth(http.HandlerFunc(a.handleCreateApproval)))
	mux.Handle("GET /v1/approvals", a.withAuth(http.HandlerFunc(a.handleListApprovals)))
	mux.Handle("GET /v1/approvals/{approval_id}", a.withAuth(http.HandlerFunc(a.handleGetApproval)))
	mux.Handle("POST /v1/approvals/{approval_action...}", a.withAuth(http.HandlerFunc(a.handleApprovalAction)))

	return mux
}
