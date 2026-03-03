package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/bus"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/gateway"
	"github.com/similarityyoung/simiclaw/pkg/idempotency"
	"github.com/similarityyoung/simiclaw/pkg/model"
	"github.com/similarityyoung/simiclaw/pkg/outbound"
	"github.com/similarityyoung/simiclaw/pkg/runner"
	"github.com/similarityyoung/simiclaw/pkg/runtime"
	"github.com/similarityyoung/simiclaw/pkg/store"
)

type App struct {
	Cfg       config.Config
	Bus       *bus.MessageBus
	Gateway   *gateway.Service
	Events    *runtime.EventRepo
	Sessions  *store.SessionStore
	StoreLoop *store.StoreLoop
	EventLoop *runtime.EventLoop
	Handler   http.Handler
}

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
		Cfg:       cfg,
		Bus:       eventBus,
		Gateway:   g,
		Events:    eventRepo,
		Sessions:  sessions,
		StoreLoop: storeLoop,
		EventLoop: eventLoop,
	}
	app.Handler = app.routes()
	return app, nil
}

func (a *App) Start() {
	a.StoreLoop.Start()
	a.EventLoop.Start()
}

func (a *App) Stop() {
	a.Bus.Close()
	a.EventLoop.StopAfterDrain()
	a.StoreLoop.Stop()
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"status": "ok"})
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		paths := []string{
			filepath.Join(a.Cfg.Workspace, "runtime", "sessions.json"),
			filepath.Join(a.Cfg.Workspace, "runtime", "sessions"),
			filepath.Join(a.Cfg.Workspace, "runtime", "runs"),
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err != nil {
				writeJSON(w, 503, map[string]any{"status": "not_ready", "error": err.Error()})
				return
			}
		}
		writeJSON(w, 200, map[string]any{"status": "ready", "queue_depth": a.Bus.InboundDepth(), "time": time.Now().UTC().Format(time.RFC3339Nano)})
	})

	mux.Handle("POST /v1/events:ingest", a.withAuth(http.HandlerFunc(a.handleIngest)))
	mux.Handle("GET /v1/events/{event_id}", a.withAuth(http.HandlerFunc(a.handleGetEvent)))

	return mux
}

func (a *App) withAuth(next http.Handler) http.Handler {
	if a.Cfg.APIKey == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")) != a.Cfg.APIKey {
			writeAPIError(w, &gateway.APIError{StatusCode: 401, Code: model.ErrorCodeUnauthorized, Message: "missing or invalid api key"})
			return
		}
		next.ServeHTTP(w, r)
	})
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
