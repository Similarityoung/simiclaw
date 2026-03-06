package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"google.golang.org/adk/agent"
	adkmodel "google.golang.org/adk/model"
	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"

	"github.com/similarityyoung/simiclaw/pkg/adkruntime"
	"github.com/similarityyoung/simiclaw/pkg/approval"
	"github.com/similarityyoung/simiclaw/pkg/bus"
	"github.com/similarityyoung/simiclaw/pkg/config"
	runtime "github.com/similarityyoung/simiclaw/pkg/eventing"
	"github.com/similarityyoung/simiclaw/pkg/gateway"
	"github.com/similarityyoung/simiclaw/pkg/idempotency"
	"github.com/similarityyoung/simiclaw/pkg/model"
	store "github.com/similarityyoung/simiclaw/pkg/persistence"
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
	adkRuntime, err := newGatewayADKRuntime(cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize gateway adk runtime: %w", err)
	}
	adkRouter := gatewayADKSessionRouter{runtime: adkRuntime, appName: defaultGatewayADKAppName}
	approvalSvc, err := approval.NewService(cfg.Workspace, eventBus)
	if err != nil {
		return nil, err
	}
	g := gateway.NewService(cfg, idStore, sessions, eventRepo, storeLoop, adkRouter)

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
		Approvals:   approvalSvc,
	}
	app.StoreLoop.Start()
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

func (r gatewayADKSessionRouter) RunIngest(ctx context.Context, req model.IngestRequest, sessionKey, sessionID string) (gateway.ADKRunResult, error) {
	result := gateway.ADKRunResult{}
	now := time.Now().UTC()
	runID := nextADKID("run", now)
	result.RunID = runID
	if isNoReplyPayload(req.Payload.Type) {
		result.RunMode = model.RunModeNoReply
		result.SuppressOutput = true
		content := strings.TrimSpace(req.Payload.Text)
		if content == "" {
			content = req.Payload.Type
		}
		result.Entries = []model.SessionEntry{{
			Type:    "system",
			EntryID: nextADKID("e_sys", now),
			RunID:   runID,
			Content: content,
		}}
		result.Trace = model.RunTrace{
			RunID:      runID,
			SessionKey: sessionKey,
			SessionID:  sessionID,
			RunMode:    result.RunMode,
			Actions:    []model.Action{},
			StartedAt:  now,
			FinishedAt: time.Now().UTC(),
		}
		return result, nil
	}
	result.RunMode = model.RunModeNormal
	userID := adkRouteUserID(req, sessionKey)
	msg := genai.NewContentFromText(strings.TrimSpace(req.Payload.Text), genai.RoleUser)
	entries := []model.SessionEntry{{
		Type:    "user",
		EntryID: nextADKID("e_user", now),
		RunID:   runID,
		Content: strings.TrimSpace(req.Payload.Text),
	}}
	var assistantText strings.Builder
	for evt, err := range r.runtime.Runner().Run(ctx, userID, sessionID, msg, agent.RunConfig{StreamingMode: agent.StreamingModeNone}) {
		if err != nil {
			return gateway.ADKRunResult{}, err
		}
		if evt == nil || evt.Content == nil || evt.Author == "user" {
			continue
		}
		if !evt.IsFinalResponse() {
			continue
		}
		for _, part := range evt.Content.Parts {
			if part != nil && strings.TrimSpace(part.Text) != "" {
				if assistantText.Len() > 0 {
					assistantText.WriteByte('\n')
				}
				assistantText.WriteString(strings.TrimSpace(part.Text))
			}
		}
	}
	if assistantText.Len() > 0 {
		result.OutboundBody = assistantText.String()
		entries = append(entries, model.SessionEntry{
			Type:    "assistant",
			EntryID: nextADKID("e_asst", time.Now().UTC()),
			RunID:   runID,
			Content: result.OutboundBody,
		})
	}
	result.Entries = entries
	result.Trace = model.RunTrace{
		RunID:      runID,
		SessionKey: sessionKey,
		SessionID:  sessionID,
		RunMode:    result.RunMode,
		Actions:    []model.Action{},
		StartedAt:  now,
		FinishedAt: time.Now().UTC(),
	}
	return result, nil
}

var adkIDSeq atomic.Uint64

func nextADKID(prefix string, now time.Time) string {
	n := adkIDSeq.Add(1)
	return fmt.Sprintf("%s_%d_%04d", prefix, now.UnixNano(), n)
}

func isNoReplyPayload(payloadType string) bool {
	switch strings.TrimSpace(payloadType) {
	case "memory_flush", "compaction", "cron_fire":
		return true
	default:
		return false
	}
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
	var llm adkmodel.LLM = adkruntime.LocalEchoModel{}
	return adkruntime.NewRuntime(adkruntime.Config{
		Workspace: cfg.Workspace,
		AppName:   defaultGatewayADKAppName,
		LLM:       llm,
	})
}

// Start 启动存储循环和事件循环，开始处理系统内消息。
func (a *App) Start() {
}

// Stop 按顺序关闭总线与后台循环，尽量完成剩余任务后退出。
func (a *App) Stop() {
	a.Bus.Close()
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
