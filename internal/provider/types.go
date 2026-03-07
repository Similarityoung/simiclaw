package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/model"
	"github.com/similarityyoung/simiclaw/pkg/tools"
)

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []model.ToolCall `json:"tool_calls,omitempty"`
}

type ToolDefinition struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Parameters  tools.ParameterSchema `json:"parameters"`
}

type ChatRequest struct {
	Model    string           `json:"model"`
	Messages []ChatMessage    `json:"messages"`
	Tools    []ToolDefinition `json:"tools,omitempty"`
}

type ChatResult struct {
	Text              string           `json:"text"`
	ToolCalls         []model.ToolCall `json:"tool_calls,omitempty"`
	FinishReason      string           `json:"finish_reason"`
	RawFinishReason   string           `json:"raw_finish_reason"`
	Usage             Usage            `json:"usage"`
	Provider          string           `json:"provider"`
	Model             string           `json:"model"`
	ProviderRequestID string           `json:"provider_request_id"`
}

type LLMProvider interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResult, error)
}

type Factory struct {
	defaultModel string
	providers    map[string]LLMProvider
}

func NewFactory(cfg config.LLMConfig) (*Factory, error) {
	out := &Factory{
		defaultModel: cfg.DefaultModel,
		providers:    make(map[string]LLMProvider, len(cfg.Providers)),
	}
	for name, providerCfg := range cfg.Providers {
		provider, err := newProvider(name, providerCfg)
		if err != nil {
			return nil, err
		}
		out.providers[name] = provider
	}
	if _, _, err := out.Resolve(cfg.DefaultModel); err != nil {
		return nil, err
	}
	return out, nil
}

func (f *Factory) DefaultModel() string {
	return f.defaultModel
}

func (f *Factory) Resolve(model string) (LLMProvider, string, error) {
	prefix, actualModel, ok := strings.Cut(strings.TrimSpace(model), "/")
	if !ok || prefix == "" || actualModel == "" {
		return nil, "", fmt.Errorf("model %q must use provider/model format", model)
	}
	provider, ok := f.providers[prefix]
	if !ok {
		return nil, "", fmt.Errorf("unknown provider %q", prefix)
	}
	return provider, actualModel, nil
}
