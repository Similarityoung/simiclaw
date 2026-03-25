package httpserver

import (
	"context"
	"net/http"

	httpingest "github.com/similarityyoung/simiclaw/internal/http/ingest"
	httpmiddleware "github.com/similarityyoung/simiclaw/internal/http/middleware"
	httpquery "github.com/similarityyoung/simiclaw/internal/http/query"
	httpstream "github.com/similarityyoung/simiclaw/internal/http/stream"
	runtimeevents "github.com/similarityyoung/simiclaw/internal/runtime/events"
)

type Server struct {
	readiness Readiness
	handler   http.Handler
}

type Readiness interface {
	ReadyState(ctx context.Context) (map[string]any, error)
}

func New(apiKey string, command httpingest.Gateway, query httpquery.Query, readiness Readiness, streamObserver runtimeevents.StreamObserver) *Server {
	server := &Server{readiness: readiness}
	write := httpingest.NewHandler(command)
	read := httpquery.NewHandlers(query)
	stream := httpstream.NewHandlers(command, streamObserver)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.handleHealthz)
	mux.HandleFunc("GET /readyz", server.handleReadyz)

	mux.Handle("POST /v1/chat:stream", httpmiddleware.WithAPIKey(apiKey, http.HandlerFunc(stream.HandleChatStream)))
	mux.Handle("POST /v1/events:ingest", httpmiddleware.WithAPIKey(apiKey, http.HandlerFunc(write.HandleIngest)))
	mux.Handle("GET /v1/events/{event_id}", httpmiddleware.WithAPIKey(apiKey, http.HandlerFunc(read.HandleGetEvent)))
	mux.Handle("GET /v1/events", httpmiddleware.WithAPIKey(apiKey, http.HandlerFunc(read.HandleListEvents)))
	mux.Handle("GET /v1/events:lookup", httpmiddleware.WithAPIKey(apiKey, http.HandlerFunc(read.HandleLookupEvent)))
	mux.Handle("GET /v1/runs", httpmiddleware.WithAPIKey(apiKey, http.HandlerFunc(read.HandleListRuns)))
	mux.Handle("GET /v1/runs/{run_id}", httpmiddleware.WithAPIKey(apiKey, http.HandlerFunc(read.HandleGetRun)))
	mux.Handle("GET /v1/runs/{run_id}/trace", httpmiddleware.WithAPIKey(apiKey, http.HandlerFunc(read.HandleGetRunTrace)))
	mux.Handle("GET /v1/sessions", httpmiddleware.WithAPIKey(apiKey, http.HandlerFunc(read.HandleListSessions)))
	mux.Handle("GET /v1/sessions/{session_key}", httpmiddleware.WithAPIKey(apiKey, http.HandlerFunc(read.HandleGetSession)))
	mux.Handle("GET /v1/sessions/{session_key}/history", httpmiddleware.WithAPIKey(apiKey, http.HandlerFunc(read.HandleGetSessionHistory)))

	server.handler = mux
	return server
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	httpquery.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	state, err := s.readiness.ReadyState(r.Context())
	if err != nil {
		httpquery.WriteJSON(w, http.StatusServiceUnavailable, state)
		return
	}
	httpquery.WriteJSON(w, http.StatusOK, state)
}
