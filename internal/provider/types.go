package provider

import (
	"fmt"
	"strings"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
)

type Usage = kernel.Usage
type ChatMessage = kernel.ModelMessage
type ToolDefinition = kernel.ToolDefinition
type ChatRequest = kernel.ModelRequest
type ChatResult = kernel.ModelResult
type StreamSink = kernel.ModelStreamSink
type LLMProvider = kernel.ModelInvoker

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

func (f *Factory) Resolve(model string) (kernel.ModelInvoker, string, error) {
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
