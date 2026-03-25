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
	Cfg       config.Config
	DB        *store.DB
	Gateway   *gateway.Service
	EventLoop *runtime.EventLoop
	Host      *runtime.HostControl
	Readiness *runtime.ReadinessProbe
	Telegram  *telegramchannel.Runtime
	StreamHub *runtimeevents.Hub
	Handler   http.Handler
}

var ErrStartup = errors.New("bootstrap startup failed")

type capabilityBundle struct {
	registry  *tools.Registry
	providers *provider.Factory
	payloads  *runtimepayload.Registry
}

type stateBundle struct {
	queryRepo    *storequeries.Repository
	runtimeRepo  *storetx.RuntimeRepository
	queryService *querysvc.Service
}

type runtimeBundle struct {
	streamHub      *runtimeevents.Hub
	eventLoop      *runtime.EventLoop
	streamObserver runtimeevents.StreamObserver
	host           *runtime.HostControl
	readiness      *runtime.ReadinessProbe
}

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
	capabilities, err := newCapabilityBundle(cfg)
	if err != nil {
		logger.Error("provider factory failed", logging.Error(err))
		_ = db.Close()
		return nil, err
	}
	state := newStateBundle(db)
	runtimeServices := newRuntimeBundle(cfg, state, capabilities)
	gatewayService := gateway.NewService(
		cfg.TenantID,
		state.runtimeRepo,
		runtimeServices.eventLoop,
		gatewaybindings.NewResolver(cfg.TenantID, gatewaySessionLookup{scopes: db, sessions: state.queryRepo}),
		gatewayrouting.NewService(capabilities.payloads),
		cfg.RateLimitTenantRPS,
		cfg.RateLimitTenantBurst,
		cfg.RateLimitSessionRPS,
		cfg.RateLimitSessionBurst,
	)

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

	runtimeServices = runtimeServices.attachProcessRuntime(cfg, state, gatewayService, telegramRuntime)
	server := httpserver.New(httpserver.Dependencies{
		APIKey:         cfg.APIKey,
		Command:        gatewayService,
		Query:          state.queryService,
		Readiness:      runtimeServices.readiness,
		StreamObserver: runtimeServices.streamObserver,
	})
	logger.Info("application assembled")
	return &App{
		Cfg:       cfg,
		DB:        db,
		Gateway:   gatewayService,
		EventLoop: runtimeServices.eventLoop,
		Host:      runtimeServices.host,
		Readiness: runtimeServices.readiness,
		Telegram:  telegramRuntime,
		StreamHub: runtimeServices.streamHub,
		Handler:   server.Handler(),
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
	if err := a.Host.Start(ctx); err != nil {
		logger.Error("runtime host start failed", logging.Error(err))
		return fmt.Errorf("%w: %w", ErrStartup, err)
	}
	logger.Info("runtime host started")
	return nil
}

func (a *App) Stop() {
	logger := logging.L("bootstrap").With(
		logging.String("workspace", a.Cfg.Workspace),
		logging.String("addr", a.Cfg.ListenAddr),
	)
	logger.Info("application stopping")
	a.Host.Stop()
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

func newCapabilityBundle(cfg config.Config) (capabilityBundle, error) {
	registry := tools.NewRegistry()
	tools.RegisterBuiltins(registry)
	tools.RegisterWebSearch(registry, tools.WebSearchOptions{
		Timeout:    cfg.WebSearch.Timeout.Duration,
		MaxResults: cfg.WebSearch.MaxResults,
	})
	providers, err := provider.NewFactory(cfg.LLM)
	if err != nil {
		return capabilityBundle{}, err
	}
	return capabilityBundle{
		registry:  registry,
		providers: providers,
		payloads:  runtimepayload.NewBuiltinRegistry(),
	}, nil
}

func newStateBundle(db *store.DB) stateBundle {
	queryRepo := storequeries.NewRepository(db)
	return stateBundle{
		queryRepo:    queryRepo,
		runtimeRepo:  storetx.NewRuntimeRepository(db),
		queryService: querysvc.NewService(queryRepo, queryRepo, queryRepo),
	}
}

func newRuntimeBundle(cfg config.Config, state stateBundle, capabilities capabilityBundle) runtimeBundle {
	streamHub := runtimeevents.NewHub()
	run := runner.NewProviderRunner(cfg.Workspace, state.queryRepo, capabilities.registry, capabilities.providers, capabilities.payloads)
	executor := runtime.NewRunnerExecutor(run, cfg.MaxToolRounds)
	return runtimeBundle{
		streamHub: streamHub,
		eventLoop: runtime.NewEventLoop(
			state.runtimeRepo,
			runtimeEventRecordView{query: state.queryRepo},
			executor,
			streamHub,
			cfg.EventQueueCapacity,
		),
		streamObserver: runtimeevents.NewObserver(streamHub, runtimeTerminalReplaySource{query: state.queryService}),
	}
}

func (b runtimeBundle) attachProcessRuntime(cfg config.Config, state stateBundle, gatewayService *gateway.Service, telegramRuntime *telegramchannel.Runtime) runtimeBundle {
	sender := outboundsender.NewRouter(outboundsender.Stdout{}, telegramRuntime)
	b.host = runtime.NewHostControl(cfg, state.runtimeRepo, newRuntimeEventIngestor(gatewayService), b.eventLoop, sender)
	extraHeartbeats := []string(nil)
	if telegramRuntime != nil {
		extraHeartbeats = append(extraHeartbeats, "telegram_polling")
	}
	b.readiness = runtime.NewReadinessProbe(state.runtimeRepo, b.host, extraHeartbeats...)
	return b
}
