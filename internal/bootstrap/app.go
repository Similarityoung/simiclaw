package bootstrap

import (
	"context"
	"net/http"
	"time"

	"github.com/similarityyoung/simiclaw/internal/httpapi"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/gateway"
	"github.com/similarityyoung/simiclaw/pkg/outbound"
	"github.com/similarityyoung/simiclaw/pkg/provider"
	"github.com/similarityyoung/simiclaw/pkg/runner"
	"github.com/similarityyoung/simiclaw/pkg/runtime"
	"github.com/similarityyoung/simiclaw/pkg/store"
	"github.com/similarityyoung/simiclaw/pkg/tools"
)

type App struct {
	Cfg        config.Config
	DB         *store.DB
	Gateway    *gateway.Service
	EventLoop  *runtime.EventLoop
	Supervisor *runtime.Supervisor
	Handler    http.Handler
}

func NewApp(cfg config.Config) (*App, error) {
	db, err := store.Open(cfg.Workspace, cfg.DBBusyTimeout.Duration)
	if err != nil {
		return nil, err
	}
	registry := tools.NewRegistry()
	tools.RegisterBuiltins(registry)
	providers, err := provider.NewFactory(cfg.LLM)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	run := runner.NewProviderRunner(cfg.Workspace, db, registry, providers)
	eventLoop := runtime.NewEventLoop(db, run, cfg.EventQueueCapacity, cfg.MaxToolRounds)
	supervisor := runtime.NewSupervisor(cfg, db, eventLoop, outbound.StdoutSender{})
	gatewayService := gateway.NewService(cfg, db, eventLoop)
	server := httpapi.New(cfg, db, gatewayService, supervisor)
	return &App{
		Cfg:        cfg,
		DB:         db,
		Gateway:    gatewayService,
		EventLoop:  eventLoop,
		Supervisor: supervisor,
		Handler:    server.Handler(),
	}, nil
}

func (a *App) Start() {
	a.Supervisor.Start()
}

func (a *App) Stop() {
	a.Supervisor.Stop()
	_ = a.DB.Close()
}

func (a *App) RunHTTPServer(ctx context.Context) error {
	a.Start()
	defer a.Stop()

	srv := &http.Server{Addr: a.Cfg.ListenAddr, Handler: a.Handler}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	return srv.ListenAndServe()
}
