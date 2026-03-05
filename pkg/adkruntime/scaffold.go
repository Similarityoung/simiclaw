package adkruntime

import (
	"fmt"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
)

type Config struct {
	Workspace string
	AppName   string
	RootAgent agent.Agent
}

type Runtime struct {
	cfg            Config
	runner         *runner.Runner
	sessionService session.Service
	memoryService  memory.Service
}

func NewRuntime(cfg Config) (*Runtime, error) {
	sessionService := session.InMemoryService()
	memoryService := memory.InMemoryService()

	r, err := runner.New(runner.Config{
		AppName:        cfg.AppName,
		Agent:          cfg.RootAgent,
		SessionService: sessionService,
		MemoryService:  memoryService,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize adk runner: %w", err)
	}

	return &Runtime{
		cfg:            cfg,
		runner:         r,
		sessionService: sessionService,
		memoryService:  memoryService,
	}, nil
}

func (r *Runtime) Config() Config {
	return r.cfg
}

func (r *Runtime) Runner() *runner.Runner {
	return r.runner
}

func (r *Runtime) SessionService() session.Service {
	return r.sessionService
}

func (r *Runtime) MemoryService() memory.Service {
	return r.memoryService
}
