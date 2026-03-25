package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	telegramchannel "github.com/similarityyoung/simiclaw/internal/channels/telegram"
	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/gateway"
	gatewaybindings "github.com/similarityyoung/simiclaw/internal/gateway/bindings"
	gatewayrouting "github.com/similarityyoung/simiclaw/internal/gateway/routing"
	httpserver "github.com/similarityyoung/simiclaw/internal/http"
	outboundsender "github.com/similarityyoung/simiclaw/internal/outbound/sender"
	"github.com/similarityyoung/simiclaw/internal/provider"
	querysvc "github.com/similarityyoung/simiclaw/internal/query"
	"github.com/similarityyoung/simiclaw/internal/runner"
	"github.com/similarityyoung/simiclaw/internal/runtime"
	runtimeevents "github.com/similarityyoung/simiclaw/internal/runtime/events"
	runtimepayload "github.com/similarityyoung/simiclaw/internal/runtime/payload"
	"github.com/similarityyoung/simiclaw/internal/store"
	storequeries "github.com/similarityyoung/simiclaw/internal/store/queries"
	storetx "github.com/similarityyoung/simiclaw/internal/store/tx"
	"github.com/similarityyoung/simiclaw/internal/tools"
	"github.com/similarityyoung/simiclaw/pkg/logging"
)

type App struct {
	Cfg        config.Config
	DB         *store.DB
	Gateway    *gateway.Service
	EventLoop  *runtime.EventLoop
	Supervisor *runtime.Supervisor
	Telegram   *telegramchannel.Runtime
	StreamHub  *runtimeevents.Hub
	Handler    http.Handler
}

var ErrStartup = errors.New("bootstrap startup failed")

// NewApp assembles the process composition root.
//
// Ownership stays in the underlying Surface, Runtime, Context/State, and
// Capability packages; bootstrap only wires them together and manages process
// lifecycle concerns.
func NewApp(cfg config.Config) (*App, error) {
	logger := logging.L("bootstrap").With(
		logging.String("workspace", cfg.Workspace),
		logging.String("addr", cfg.ListenAddr),
	)
	db, err := store.Open(cfg.Workspace, cfg.DBBusyTimeout.Duration)
	if err != nil {
		logger.Error("open database failed", logging.Error(err))
		return nil, err
	}
	logger.Info("database opened")
	registry := tools.NewRegistry()
	tools.RegisterBuiltins(registry)
	tools.RegisterWebSearch(registry, tools.WebSearchOptions{
		Timeout:    cfg.WebSearch.Timeout.Duration,
		MaxResults: cfg.WebSearch.MaxResults,
	})
	providers, err := provider.NewFactory(cfg.LLM)
	if err != nil {
		logger.Error("provider factory failed", logging.Error(err))
		_ = db.Close()
		return nil, err
	}
	streamHub := runtimeevents.NewHub()
	payloads := runtimepayload.NewBuiltinRegistry()
	queryRepo := storequeries.NewRepository(db)
	run := runner.NewProviderRunner(cfg.Workspace, queryRepo, registry, providers, payloads)
	runtimeRepo := storetx.NewRuntimeRepository(db)
	executor := runtime.NewRunnerExecutor(run, cfg.MaxToolRounds)
	eventLoop := runtime.NewEventLoop(runtimeRepo, executor, streamHub, cfg.EventQueueCapacity)
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
	queryService := querysvc.NewService(queryRepo)

	var telegramRuntime *telegramchannel.Runtime
	if cfg.Channels.Telegram.Enabled {
		telegramRuntime, err = telegramchannel.NewRuntime(cfg.Channels.Telegram, db, gatewayService)
		if err != nil {
			logger.Error("telegram runtime init failed", logging.Error(err))
			_ = db.Close()
			return nil, err
		}
		logger.Info("telegram runtime configured")
	}

	sender := outboundsender.NewRouter(outboundsender.Stdout{}, telegramRuntime)
	supervisor := runtime.NewSupervisor(cfg, runtimeRepo, runtimeRepo, newRuntimeEventIngestor(gatewayService), eventLoop, sender)
	server := httpserver.New(cfg, gatewayService, queryService, supervisor, streamHub)
	logger.Info("application assembled")
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

func (a *App) Start(ctx context.Context) error {
	logger := logging.L("bootstrap").With(
		logging.String("workspace", a.Cfg.Workspace),
		logging.String("addr", a.Cfg.ListenAddr),
	)
	if a.Telegram != nil {
		logger.Info("telegram runtime starting")
		if err := a.Telegram.Start(); err != nil {
			logger.Error("telegram runtime start failed", logging.Error(err))
			return fmt.Errorf("%w: %w", ErrStartup, err)
		}
		logger.Info("telegram runtime started")
	}
	if err := a.Supervisor.Start(ctx); err != nil {
		logger.Error("runtime supervisor start failed", logging.Error(err))
		return fmt.Errorf("%w: %w", ErrStartup, err)
	}
	logger.Info("runtime supervisor started")
	return nil
}

func (a *App) Stop() {
	logger := logging.L("bootstrap").With(
		logging.String("workspace", a.Cfg.Workspace),
		logging.String("addr", a.Cfg.ListenAddr),
	)
	logger.Info("application stopping")
	a.Supervisor.Stop()
	if a.Telegram != nil {
		a.Telegram.Stop()
	}
	_ = a.DB.Close()
	logger.Info("application stopped")
}

func (a *App) RunHTTPServer(ctx context.Context) error {
	if err := a.Start(ctx); err != nil {
		return err
	}
	defer a.Stop()

	srv := &http.Server{Addr: a.Cfg.ListenAddr, Handler: a.Handler}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	return srv.ListenAndServe()
}
