package adkruntime

import (
	"errors"
	"fmt"

	"github.com/similarityyoung/simiclaw/pkg/tools"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	adktool "google.golang.org/adk/tool"
)

type Config struct {
	Workspace string
	AppName   string
	RootAgent agent.Agent
	LLM       model.LLM
}

type Runtime struct {
	cfg            Config
	runner         *runner.Runner
	sessionService session.Service
	memoryService  memory.Service
}

func NewRuntime(cfg Config) (*Runtime, error) {
	rootAgent := cfg.RootAgent
	if rootAgent == nil {
		var err error
		rootAgent, err = NewPrimaryLlmAgent(PrimaryLlmAgentConfig{Model: cfg.LLM, Workspace: cfg.Workspace})
		if err != nil {
			return nil, fmt.Errorf("initialize root agent: %w", err)
		}
	}

	sessionService := session.InMemoryService()
	memoryService := memory.InMemoryService()

	r, err := runner.New(runner.Config{
		AppName:        cfg.AppName,
		Agent:          rootAgent,
		SessionService: sessionService,
		MemoryService:  memoryService,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize adk runner: %w", err)
	}

	cfg.RootAgent = rootAgent

	return &Runtime{
		cfg:            cfg,
		runner:         r,
		sessionService: sessionService,
		memoryService:  memoryService,
	}, nil
}

const (
	DefaultPrimaryLlmAgentName        = "simiclaw_primary"
	DefaultPrimaryLlmAgentDescription = "Primary SimiClaw ADK LLM agent"
	PrimaryLlmAgentInstruction        = "You are SimiClaw's primary runtime agent. Handle user intents accurately, use tools only when they materially improve outcomes, and avoid unsafe or irreversible side effects unless explicitly justified by the task. Respect high-level runtime semantics: operational internal events such as memory/compaction/cron flows may be no-reply and should not produce user-facing replies unless explicitly requested. Keep outputs concise, actionable, and grounded in available context."
)

type PrimaryLlmAgentConfig struct {
	Model       model.LLM
	Name        string
	Description string
	Workspace   string
}

func NewPrimaryLlmAgent(cfg PrimaryLlmAgentConfig) (agent.Agent, error) {
	if cfg.Model == nil {
		return nil, errors.New("llm model is required")
	}

	fileReadTool, err := tools.NewFileReadTool(cfg.Workspace)
	if err != nil {
		return nil, fmt.Errorf("initialize primary llm agent tool file_read: %w", err)
	}

	fileWriteTool, err := tools.NewFileWriteTool(cfg.Workspace)
	if err != nil {
		return nil, fmt.Errorf("initialize primary llm agent tool file_write: %w", err)
	}

	fileEditTool, err := tools.NewFileEditTool(cfg.Workspace)
	if err != nil {
		return nil, fmt.Errorf("initialize primary llm agent tool file_edit: %w", err)
	}

	bashTool, err := tools.NewBashTool(cfg.Workspace)
	if err != nil {
		return nil, fmt.Errorf("initialize primary llm agent tool bash: %w", err)
	}

	name := cfg.Name
	if name == "" {
		name = DefaultPrimaryLlmAgentName
	}

	description := cfg.Description
	if description == "" {
		description = DefaultPrimaryLlmAgentDescription
	}

	llmAgent, err := llmagent.New(llmagent.Config{
		Name:        name,
		Description: description,
		Model:       cfg.Model,
		Instruction: PrimaryLlmAgentInstruction,
		Tools:       []adktool.Tool{fileReadTool, fileWriteTool, fileEditTool, bashTool},
	})
	if err != nil {
		return nil, fmt.Errorf("initialize primary llm agent: %w", err)
	}

	return llmAgent, nil
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
