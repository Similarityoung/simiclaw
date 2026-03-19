package httpserver

import (
	"net/http"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/gateway"
	httpingest "github.com/similarityyoung/simiclaw/internal/http/ingest"
	httpmiddleware "github.com/similarityyoung/simiclaw/internal/http/middleware"
	httpquery "github.com/similarityyoung/simiclaw/internal/http/query"
	httpstream "github.com/similarityyoung/simiclaw/internal/http/stream"
	querysvc "github.com/similarityyoung/simiclaw/internal/query"
	"github.com/similarityyoung/simiclaw/internal/runtime"
	"github.com/similarityyoung/simiclaw/internal/streaming"
)

type Server struct {
	supervisor *runtime.Supervisor
	handler    http.Handler
}

func New(cfg config.Config, gatewayService *gateway.Service, queryService *querysvc.Service, supervisor *runtime.Supervisor, streamHub *streaming.Hub) *Server {
	server := &Server{supervisor: supervisor}
	write := httpingest.NewHandler(gatewayService)
	read := httpquery.NewHandlers(queryService)
	stream := httpstream.NewHandlers(gatewayService, queryService, streamHub)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.handleHealthz)
	mux.HandleFunc("GET /readyz", server.handleReadyz)

	mux.Handle("POST /v1/chat:stream", httpmiddleware.WithAPIKey(cfg.APIKey, http.HandlerFunc(stream.HandleChatStream)))
	mux.Handle("POST /v1/events:ingest", httpmiddleware.WithAPIKey(cfg.APIKey, http.HandlerFunc(write.HandleIngest)))
	mux.Handle("GET /v1/events/{event_id}", httpmiddleware.WithAPIKey(cfg.APIKey, http.HandlerFunc(read.HandleGetEvent)))
	mux.Handle("GET /v1/events", httpmiddleware.WithAPIKey(cfg.APIKey, http.HandlerFunc(read.HandleListEvents)))
	mux.Handle("GET /v1/events:lookup", httpmiddleware.WithAPIKey(cfg.APIKey, http.HandlerFunc(read.HandleLookupEvent)))
	mux.Handle("GET /v1/runs", httpmiddleware.WithAPIKey(cfg.APIKey, http.HandlerFunc(read.HandleListRuns)))
	mux.Handle("GET /v1/runs/{run_id}", httpmiddleware.WithAPIKey(cfg.APIKey, http.HandlerFunc(read.HandleGetRun)))
	mux.Handle("GET /v1/runs/{run_id}/trace", httpmiddleware.WithAPIKey(cfg.APIKey, http.HandlerFunc(read.HandleGetRunTrace)))
	mux.Handle("GET /v1/sessions", httpmiddleware.WithAPIKey(cfg.APIKey, http.HandlerFunc(read.HandleListSessions)))
	mux.Handle("GET /v1/sessions/{session_key}", httpmiddleware.WithAPIKey(cfg.APIKey, http.HandlerFunc(read.HandleGetSession)))
	mux.Handle("GET /v1/sessions/{session_key}/history", httpmiddleware.WithAPIKey(cfg.APIKey, http.HandlerFunc(read.HandleGetSessionHistory)))

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
	state, err := s.supervisor.ReadyState(r.Context())
	if err != nil {
		httpquery.WriteJSON(w, http.StatusServiceUnavailable, state)
		return
	}
	httpquery.WriteJSON(w, http.StatusOK, state)
}
