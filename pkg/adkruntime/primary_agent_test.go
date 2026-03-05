package adkruntime

import (
	"context"
	"iter"
	"strings"
	"testing"

	"google.golang.org/adk/model"
)

type stubLLM struct{}

func (stubLLM) Name() string {
	return "stub-llm"
}

func (stubLLM) GenerateContent(context.Context, *model.LLMRequest, bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {}
}

func TestNewPrimaryLlmAgentUsesDefaultIdentity(t *testing.T) {
	agt, err := NewPrimaryLlmAgent(PrimaryLlmAgentConfig{Model: stubLLM{}, Workspace: t.TempDir()})
	if err != nil {
		t.Fatalf("expected primary llm agent creation to succeed, got error: %v", err)
	}

	if agt.Name() != DefaultPrimaryLlmAgentName {
		t.Fatalf("expected default name %q, got %q", DefaultPrimaryLlmAgentName, agt.Name())
	}
	if agt.Description() != DefaultPrimaryLlmAgentDescription {
		t.Fatalf("expected default description %q, got %q", DefaultPrimaryLlmAgentDescription, agt.Description())
	}
}

func TestNewPrimaryLlmAgentReturnsErrorWhenModelMissing(t *testing.T) {
	_, err := NewPrimaryLlmAgent(PrimaryLlmAgentConfig{})
	if err == nil {
		t.Fatalf("expected error when llm model is missing")
	}
	if !strings.Contains(err.Error(), "llm model is required") {
		t.Fatalf("expected llm model validation error, got: %v", err)
	}
}

func TestNewPrimaryLlmAgentReturnsErrorWhenWorkspaceMissing(t *testing.T) {
	_, err := NewPrimaryLlmAgent(PrimaryLlmAgentConfig{Model: stubLLM{}})
	if err == nil {
		t.Fatalf("expected error when workspace is missing")
	}
	if !strings.Contains(err.Error(), "initialize primary llm agent tool file_read") {
		t.Fatalf("expected file_read tool initialization error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid_argument: workspace is required") {
		t.Fatalf("expected workspace validation error, got: %v", err)
	}
}
