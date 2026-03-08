package httpapi

import "net/http"

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)

	mux.Handle("POST /v1/chat:stream", s.withAuth(http.HandlerFunc(s.handleChatStream)))
	mux.Handle("POST /v1/events:ingest", s.withAuth(http.HandlerFunc(s.handleIngest)))
	mux.Handle("GET /v1/events/{event_id}", s.withAuth(http.HandlerFunc(s.handleGetEvent)))
	mux.Handle("GET /v1/events", s.withAuth(http.HandlerFunc(s.handleListEvents)))
	mux.Handle("GET /v1/events:lookup", s.withAuth(http.HandlerFunc(s.handleLookupEvent)))
	mux.Handle("GET /v1/runs", s.withAuth(http.HandlerFunc(s.handleListRuns)))
	mux.Handle("GET /v1/runs/{run_id}", s.withAuth(http.HandlerFunc(s.handleGetRun)))
	mux.Handle("GET /v1/runs/{run_id}/trace", s.withAuth(http.HandlerFunc(s.handleGetRunTrace)))
	mux.Handle("GET /v1/sessions", s.withAuth(http.HandlerFunc(s.handleListSessions)))
	mux.Handle("GET /v1/sessions/{session_key}", s.withAuth(http.HandlerFunc(s.handleGetSession)))
	mux.Handle("GET /v1/sessions/{session_key}/history", s.withAuth(http.HandlerFunc(s.handleGetSessionHistory)))

	return mux
}
