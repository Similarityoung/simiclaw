package bootstrap

import (
	"context"
	"net/http"
	"time"

	telegramchannel "github.com/similarityyoung/simiclaw/internal/channels/telegram"
	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/gateway"
	"github.com/similarityyoung/simiclaw/internal/httpapi"
	"github.com/similarityyoung/simiclaw/internal/ingest"
	"github.com/similarityyoung/simiclaw/internal/outbound"
	"github.com/similarityyoung/simiclaw/internal/provider"
	querysvc "github.com/similarityyoung/simiclaw/internal/query"
	"github.com/similarityyoung/simiclaw/internal/runner"
	"github.com/similarityyoung/simiclaw/internal/runtime"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/internal/streaming"
	"github.com/similarityyoung/simiclaw/internal/tools"
)

type App struct {
	Cfg        config.Config
	DB         *store.DB
	Gateway    *gateway.Service
	EventLoop  *runtime.EventLoop
	Supervisor *runtime.Supervisor
	Telegram   *telegramchannel.Runtime
	StreamHub  *streaming.Hub
	Handler    http.Handler
}

func NewApp(cfg config.Config) (*App, error) {
	db, err := store.Open(cfg.Workspace, cfg.DBBusyTimeout.Duration)
	if err != nil {
		return nil, err
	}
	registry := tools.NewRegistry()
	tools.RegisterBuiltins(registry)
	tools.RegisterWebSearch(registry, tools.WebSearchOptions{
		Timeout:    cfg.WebSearch.Timeout.Duration,
		MaxResults: cfg.WebSearch.MaxResults,
	})
	providers, err := provider.NewFactory(cfg.LLM)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	streamHub := streaming.NewHub()
	run := runner.NewProviderRunner(cfg.Workspace, db, registry, providers)
	eventLoop := runtime.NewEventLoop(db, run, streamHub, cfg.EventQueueCapacity, cfg.MaxToolRounds)
	ingestService := ingest.NewService(
		cfg.TenantID,
		db,
		eventLoop,
		ingest.NewScopeResolver(cfg.TenantID, db),
		cfg.RateLimitTenantRPS,
		cfg.RateLimitTenantBurst,
		cfg.RateLimitSessionRPS,
		cfg.RateLimitSessionBurst,
	)
	gatewayService := gateway.NewService(ingestService)
	queryService := querysvc.NewService(db)

	var telegramRuntime *telegramchannel.Runtime
	if cfg.Channels.Telegram.Enabled {
		telegramRuntime, err = telegramchannel.NewRuntime(cfg.Channels.Telegram, db, gatewayService)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
	}

	sender := outbound.NewRouterSender(outbound.StdoutSender{}, telegramRuntime)
	supervisor := runtime.NewSupervisor(cfg, db, ingestService, eventLoop, sender)
	server := httpapi.New(cfg, gatewayService, queryService, supervisor, streamHub)
	return &App{
		Cfg:        cfg,
		DB:         db,
		Gateway:    gatewayService,
		EventLoop:  eventLoop,
		Supervisor: supervisor,
		Telegram:   telegramRuntime,
		StreamHub:  streamHub,
		Handler:    server.Handler(),
	}, nil
}

func (a *App) Start() error {
	if a.Telegram != nil {
		if err := a.Telegram.Start(); err != nil {
			return err
		}
	}
	a.Supervisor.Start()
	return nil
}

func (a *App) Stop() {
	a.Supervisor.Stop()
	if a.Telegram != nil {
		a.Telegram.Stop()
	}
	_ = a.DB.Close()
}

func (a *App) RunHTTPServer(ctx context.Context) error {
	if err := a.Start(); err != nil {
		return err
	}
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
