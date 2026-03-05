package adkruntime

import (
	"iter"
	"strings"
	"testing"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/session"
)

func TestNewRuntimeInitializesADKHandles(t *testing.T) {
	rootAgent, err := agent.New(agent.Config{
		Name:        "root",
		Description: "test root agent",
		Run: func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {}
		},
	})
	if err != nil {
		t.Fatalf("expected root agent creation to succeed, got error: %v", err)
	}

	rt, err := NewRuntime(Config{
		Workspace: t.TempDir(),
		AppName:   "simiclaw-adk",
		RootAgent: rootAgent,
	})
	if err != nil {
		t.Fatalf("expected runtime initialization to succeed, got error: %v", err)
	}

	if rt.Runner() == nil {
		t.Fatalf("expected runner to be initialized")
	}
	if rt.SessionService() == nil {
		t.Fatalf("expected session service to be initialized")
	}
	if rt.MemoryService() == nil {
		t.Fatalf("expected memory service to be initialized")
	}
}

func TestNewRuntimeReturnsErrorWhenRootAgentMissing(t *testing.T) {
	_, err := NewRuntime(Config{Workspace: t.TempDir()})
	if err == nil {
		t.Fatalf("expected error when root agent and llm model are missing")
	}
	if !strings.Contains(err.Error(), "llm model is required") {
		t.Fatalf("expected llm model validation error, got: %v", err)
	}
}

func TestNewRuntimeBuildsPrimaryAgentWhenRootAgentMissing(t *testing.T) {
	rt, err := NewRuntime(Config{
		Workspace: t.TempDir(),
		AppName:   "simiclaw-adk",
		LLM:       stubLLM{},
	})
	if err != nil {
		t.Fatalf("expected runtime initialization to succeed with primary llm agent, got error: %v", err)
	}

	if rt.Config().RootAgent == nil {
		t.Fatalf("expected runtime config to include resolved root agent")
	}
	if rt.Config().RootAgent.Name() != DefaultPrimaryLlmAgentName {
		t.Fatalf("expected default root agent name %q, got %q", DefaultPrimaryLlmAgentName, rt.Config().RootAgent.Name())
	}
}

func TestNewRuntimeReturnsErrorWhenPrimaryToolsFailToInitialize(t *testing.T) {
	_, err := NewRuntime(Config{
		AppName: "simiclaw-adk",
		LLM:     stubLLM{},
	})
	if err == nil {
		t.Fatalf("expected runtime initialization to fail when workspace is missing")
	}
	if !strings.Contains(err.Error(), "initialize root agent") {
		t.Fatalf("expected root agent initialization error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "initialize primary llm agent tool file_read") {
		t.Fatalf("expected tool initialization error, got: %v", err)
	}
}
