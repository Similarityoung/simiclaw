package bootstrap

import (
	"context"
	"net/http"
	"time"

	telegramchannel "github.com/similarityyoung/simiclaw/internal/channels/telegram"
	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/gateway"
	gatewaybindings "github.com/similarityyoung/simiclaw/internal/gateway/bindings"
	gatewayrouting "github.com/similarityyoung/simiclaw/internal/gateway/routing"
	httpserver "github.com/similarityyoung/simiclaw/internal/http"
	"github.com/similarityyoung/simiclaw/internal/outbound"
	"github.com/similarityyoung/simiclaw/internal/provider"
	querysvc "github.com/similarityyoung/simiclaw/internal/query"
	"github.com/similarityyoung/simiclaw/internal/runner"
	"github.com/similarityyoung/simiclaw/internal/runtime"
	runtimepayload "github.com/similarityyoung/simiclaw/internal/runtime/payload"
	"github.com/similarityyoung/simiclaw/internal/store"
	storetx "github.com/similarityyoung/simiclaw/internal/store/tx"
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
	payloads := runtimepayload.NewBuiltinRegistry()
	run := runner.NewProviderRunner(cfg.Workspace, db, registry, providers, payloads)
	runtimeRepo := storetx.NewRuntimeRepository(db)
	executor := runtime.NewRunnerExecutor(run, cfg.MaxToolRounds, streamHub)
	eventLoop := runtime.NewEventLoop(runtimeRepo, executor, runtime.NewHubRuntimeEventSink(streamHub, runtimeRepo), cfg.EventQueueCapacity)
	gatewayService := gateway.NewService(
		cfg.TenantID,
		runtimeRepo,
		eventLoop,
		gatewaybindings.NewResolver(cfg.TenantID, runtimeRepo),
		gatewayrouting.NewService(payloads),
		cfg.RateLimitTenantRPS,
		cfg.RateLimitTenantBurst,
		cfg.RateLimitSessionRPS,
		cfg.RateLimitSessionBurst,
	)
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
	supervisor := runtime.NewSupervisor(cfg, runtimeRepo, runtimeRepo, newRuntimeEventIngestor(gatewayService), eventLoop, sender)
	server := httpserver.New(cfg, gatewayService, queryService, supervisor, streamHub)
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
