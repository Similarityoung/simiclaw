package api

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"strings"
	"time"

	adkagent "google.golang.org/adk/agent"
	adksession "google.golang.org/adk/session"

	"github.com/similarityyoung/simiclaw/pkg/adkruntime"
	"github.com/similarityyoung/simiclaw/pkg/agent"
	"github.com/similarityyoung/simiclaw/pkg/approval"
	"github.com/similarityyoung/simiclaw/pkg/bus"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/gateway"
	"github.com/similarityyoung/simiclaw/pkg/idempotency"
	"github.com/similarityyoung/simiclaw/pkg/llm"
	"github.com/similarityyoung/simiclaw/pkg/model"
	"github.com/similarityyoung/simiclaw/pkg/outbound"
	"github.com/similarityyoung/simiclaw/pkg/runner"
	"github.com/similarityyoung/simiclaw/pkg/runtime"
	"github.com/similarityyoung/simiclaw/pkg/store"
	"github.com/similarityyoung/simiclaw/pkg/tools"
)

type App struct {
	Cfg         config.Config
	ADKRuntime  *adkruntime.Runtime
	Bus         *bus.MessageBus
	Gateway     *gateway.Service
	Events      *runtime.EventRepo
	Runs        *runtime.RunRepo
	Sessions    *store.SessionStore
	Idempotency *idempotency.Store
	StoreLoop   *store.StoreLoop
	EventLoop   *runtime.EventLoop
	Approvals   *approval.Service
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
	var adkRuntime *adkruntime.Runtime
	var adkRouter gatewayADKSessionRouter
	if cfg.EnableADKGateway {
		var adkErr error
		adkRuntime, adkErr = newGatewayADKRuntime(cfg)
		if adkErr != nil {
			return nil, fmt.Errorf("initialize gateway adk runtime: %w", adkErr)
		}
		adkRouter = gatewayADKSessionRouter{runtime: adkRuntime, appName: defaultGatewayADKAppName}
	}
	approvalSvc, err := approval.NewService(cfg.Workspace, eventBus)
	if err != nil {
		return nil, err
	}
	registry := tools.NewRegistry()
	tools.RegisterBuiltins(registry)
	run := newRunner(cfg, registry)
	eventLoop := runtime.NewEventLoop(eventBus, eventRepo, run, storeLoop, outHub, approvalSvc, cfg.MaxToolRounds)
	g := gateway.NewService(cfg, eventBus, idStore, sessions, eventRepo, adkRouter)

	app := &App{
		Cfg:         cfg,
		ADKRuntime:  adkRuntime,
		Bus:         eventBus,
		Gateway:     g,
		Events:      eventRepo,
		Runs:        runRepo,
		Sessions:    sessions,
		Idempotency: idStore,
		StoreLoop:   storeLoop,
		EventLoop:   eventLoop,
		Approvals:   approvalSvc,
	}
	app.Handler = app.routes()
	return app, nil
}

const defaultGatewayADKAppName = "simiclaw-gateway-adk"

type gatewayADKSessionRouter struct {
	runtime *adkruntime.Runtime
	appName string
}

func (r gatewayADKSessionRouter) RouteIngest(ctx context.Context, req model.IngestRequest, sessionKey, sessionID string) error {
	if r.runtime == nil {
		return fmt.Errorf("runtime is nil")
	}
	userID := adkRouteUserID(req, sessionKey)
	_, err := r.runtime.SessionService().Get(ctx, &adksession.GetRequest{
		AppName:   r.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err == nil {
		return nil
	}
	_, err = r.runtime.SessionService().Create(ctx, &adksession.CreateRequest{
		AppName:   r.appName,
		UserID:    userID,
		SessionID: sessionID,
		State: map[string]any{
			"session_key": sessionKey,
		},
	})
	if err != nil {
		return fmt.Errorf("create adk session: %w", err)
	}
	return nil
}

func adkRouteUserID(req model.IngestRequest, sessionKey string) string {
	if v := strings.TrimSpace(req.Conversation.ParticipantID); v != "" {
		return v
	}
	if v := strings.TrimSpace(req.Conversation.ConversationID); v != "" {
		return v
	}
	return sessionKey
}

func newGatewayADKRuntime(cfg config.Config) (*adkruntime.Runtime, error) {
	rootAgent, err := adkagent.New(adkagent.Config{
		Name:        "simiclaw_gateway_adk_bridge",
		Description: "Gateway ADK bridge agent for side-by-side session routing",
		Run: func(ctx adkagent.InvocationContext) iter.Seq2[*adksession.Event, error] {
			return func(yield func(*adksession.Event, error) bool) {}
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create adk bridge root agent: %w", err)
	}
	return adkruntime.NewRuntime(adkruntime.Config{
		Workspace: cfg.Workspace,
		AppName:   defaultGatewayADKAppName,
		RootAgent: rootAgent,
	})
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

// newRunner constructs the appropriate Runner based on config.
// 仅当 LLMAPIKey 非空时使用 AgentRunner（真实 LLM）；否则回退到 ProcessRunner（内置规则引擎）。
func newRunner(cfg config.Config, registry *tools.Registry) runner.Runner {
	if cfg.LLMAPIKey != "" {
		llmClient := llm.New(llm.Config{
			BaseURL: cfg.LLMBaseURL,
			APIKey:  cfg.LLMAPIKey,
			Model:   cfg.LLMModel,
			Timeout: cfg.LLMTimeout.Duration,
		})
		return agent.New(agent.Config{
			Workspace: cfg.Workspace,
			LLM:       llmClient,
			Registry:  registry,
		})
	}
	return runner.NewProcessRunner(cfg.Workspace, registry)
}
