package httpapi

import (
	"net/http"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/gateway"
	querysvc "github.com/similarityyoung/simiclaw/internal/query"
	"github.com/similarityyoung/simiclaw/internal/runtime"
	"github.com/similarityyoung/simiclaw/internal/streaming"
)

type Server struct {
	cfg        config.Config
	gateway    *gateway.Service
	query      *querysvc.Service
	supervisor *runtime.Supervisor
	streamHub  *streaming.Hub
	handler    http.Handler
}

func New(cfg config.Config, gatewayService *gateway.Service, queryService *querysvc.Service, supervisor *runtime.Supervisor, streamHub *streaming.Hub) *Server {
	server := &Server{
		cfg:        cfg,
		gateway:    gatewayService,
		query:      queryService,
		supervisor: supervisor,
		streamHub:  streamHub,
	}
	server.handler = server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return s.handler
}
