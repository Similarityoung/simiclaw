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
	handler http.Handler
}

type Readiness interface {
	ReadyState(ctx context.Context) (map[string]any, error)
}

type Dependencies struct {
	APIKey         string
	Command        httpingest.Gateway
	Query          httpquery.Query
	Readiness      Readiness
	StreamObserver runtimeevents.StreamObserver
}

type serverRoutes struct {
	apiKey    string
	readiness Readiness
	write     *httpingest.Handler
	read      *httpquery.Handlers
	stream    *httpstream.Handlers
}

func New(deps Dependencies) *Server {
	routes := serverRoutes{
		apiKey:    deps.APIKey,
		readiness: deps.Readiness,
		write:     httpingest.NewHandler(deps.Command),
		read:      httpquery.NewHandlers(deps.Query),
		stream:    httpstream.NewHandlers(deps.Command, deps.StreamObserver),
	}
	return &Server{handler: routes.handler()}
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (r serverRoutes) handler() http.Handler {
	mux := http.NewServeMux()
	r.registerHealthRoutes(mux)
	r.registerCommandRoutes(mux)
	r.registerQueryRoutes(mux)
	return mux
}

func (r serverRoutes) registerHealthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", r.handleHealthz)
	mux.HandleFunc("GET /readyz", r.handleReadyz)
}

func (r serverRoutes) registerCommandRoutes(mux *http.ServeMux) {
	mux.Handle("POST /v1/chat:stream", r.protected(http.HandlerFunc(r.stream.HandleChatStream)))
	mux.Handle("POST /v1/events:ingest", r.protected(http.HandlerFunc(r.write.HandleIngest)))
}

func (r serverRoutes) registerQueryRoutes(mux *http.ServeMux) {
	mux.Handle("GET /v1/events/{event_id}", r.protected(http.HandlerFunc(r.read.HandleGetEvent)))
	mux.Handle("GET /v1/events", r.protected(http.HandlerFunc(r.read.HandleListEvents)))
	mux.Handle("GET /v1/events:lookup", r.protected(http.HandlerFunc(r.read.HandleLookupEvent)))
	mux.Handle("GET /v1/runs", r.protected(http.HandlerFunc(r.read.HandleListRuns)))
	mux.Handle("GET /v1/runs/{run_id}", r.protected(http.HandlerFunc(r.read.HandleGetRun)))
	mux.Handle("GET /v1/runs/{run_id}/trace", r.protected(http.HandlerFunc(r.read.HandleGetRunTrace)))
	mux.Handle("GET /v1/sessions", r.protected(http.HandlerFunc(r.read.HandleListSessions)))
	mux.Handle("GET /v1/sessions/{session_key}", r.protected(http.HandlerFunc(r.read.HandleGetSession)))
	mux.Handle("GET /v1/sessions/{session_key}/history", r.protected(http.HandlerFunc(r.read.HandleGetSessionHistory)))
}

func (r serverRoutes) protected(next http.Handler) http.Handler {
	return httpmiddleware.WithAPIKey(r.apiKey, next)
}

func (r serverRoutes) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	httpquery.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (r serverRoutes) handleReadyz(w http.ResponseWriter, req *http.Request) {
	state, err := r.readiness.ReadyState(req.Context())
	if err != nil {
		httpquery.WriteJSON(w, http.StatusServiceUnavailable, state)
		return
	}
	httpquery.WriteJSON(w, http.StatusOK, state)
}
