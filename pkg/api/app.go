package api

import (
	"context"
	"net/http"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/bus"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/gateway"
	"github.com/similarityyoung/simiclaw/pkg/idempotency"
	"github.com/similarityyoung/simiclaw/pkg/outbound"
	"github.com/similarityyoung/simiclaw/pkg/runner"
	"github.com/similarityyoung/simiclaw/pkg/runtime"
	"github.com/similarityyoung/simiclaw/pkg/store"
)

type App struct {
	Cfg         config.Config
	Bus         *bus.MessageBus
	Gateway     *gateway.Service
	Events      *runtime.EventRepo
	Runs        *runtime.RunRepo
	Sessions    *store.SessionStore
	Idempotency *idempotency.Store
	StoreLoop   *store.StoreLoop
	EventLoop   *runtime.EventLoop
	Handler     http.Handler
}

// NewApp 初始化工作区和核心组件，并组装可运行的应用实例。
func NewApp(cfg config.Config) (*App, error) {
	if err := store.InitWorkspace(cfg.Workspace); err != nil {
		return nil, err
	}

	idStore, err := idempotency.New(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	eventRepo, err := runtime.NewEventRepo(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	runRepo := runtime.NewRunRepo(cfg.Workspace)
	sessions, err := store.NewSessionStore(cfg.Workspace)
	if err != nil {
		return nil, err
	}

	eventBus := bus.NewMessageBus(cfg.EventQueueCapacity)
	storeLoop := store.NewStoreLoop(cfg.Workspace, sessions)
	outHub := outbound.NewHub(cfg.Workspace, outbound.StdoutSender{}, idStore)
	run := runner.NewProcessRunner()
	eventLoop := runtime.NewEventLoop(eventBus, eventRepo, run, storeLoop, outHub, cfg.MaxToolRounds)
	g := gateway.NewService(cfg, eventBus, idStore, sessions, eventRepo)

	app := &App{
		Cfg:         cfg,
		Bus:         eventBus,
		Gateway:     g,
		Events:      eventRepo,
		Runs:        runRepo,
		Sessions:    sessions,
		Idempotency: idStore,
		StoreLoop:   storeLoop,
		EventLoop:   eventLoop,
	}
	app.Handler = app.routes()
	return app, nil
}

// Start 启动存储循环和事件循环，开始处理系统内消息。
func (a *App) Start() {
	a.StoreLoop.Start()
	a.EventLoop.Start()
}

// Stop 按顺序关闭总线与后台循环，尽量完成剩余任务后退出。
func (a *App) Stop() {
	a.Bus.Close()
	a.EventLoop.StopAfterDrain()
	a.StoreLoop.Stop()
}

// RunHTTPServer 启动 HTTP 服务并在上下文取消时执行优雅关闭。
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
