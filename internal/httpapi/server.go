package httpapi

import (
	"net/http"

	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/gateway"
	"github.com/similarityyoung/simiclaw/pkg/runtime"
	"github.com/similarityyoung/simiclaw/pkg/store"
)

type Server struct {
	cfg        config.Config
	db         *store.DB
	gateway    *gateway.Service
	supervisor *runtime.Supervisor
	handler    http.Handler
}

func New(cfg config.Config, db *store.DB, gatewayService *gateway.Service, supervisor *runtime.Supervisor) *Server {
	server := &Server{
		cfg:        cfg,
		db:         db,
		gateway:    gatewayService,
		supervisor: supervisor,
	}
	server.handler = server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return s.handler
}
