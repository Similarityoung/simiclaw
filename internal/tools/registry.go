package tools

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type Registry struct {
	mu   sync.RWMutex
	defs map[string]Definition
}

func NewRegistry() *Registry {
	return &Registry{defs: map[string]Definition{}}
}

// Register adds a tool with its schema and handler.
// Callers that don't have a schema yet can use RegisterHandler.
func (r *Registry) Register(name string, schema Schema, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defs[name] = Definition{Schema: schema, Handler: handler}
}

// RegisterHandler is a convenience wrapper for tools that don't need
// a full schema (e.g., built-ins registered before schema support).
func (r *Registry) RegisterHandler(name string, handler Handler) {
	r.Register(name, Schema{Name: name}, handler)
}

func (r *Registry) Invoke(ctx context.Context, toolCtx kernel.ToolContext, name string, args map[string]any) (result kernel.ToolResult) {
	r.mu.RLock()
	d, ok := r.defs[name]
	r.mu.RUnlock()
	if !ok {
		return kernel.ToolResult{Error: &model.ErrorBlock{Code: model.ErrorCodeInvalidArgument, Message: "tool not found"}}
	}
	if ctx != nil && ctx.Err() != nil {
		return kernel.ToolResult{
			Error: kernel.ErrorBlockFromError(kernel.WrapCapabilityError("tool", name, "invoke", ctx.Err())),
		}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			result = kernel.ToolResult{
				Error: &model.ErrorBlock{
					Code:    model.ErrorCodeInternal,
					Message: fmt.Sprintf("tool panic: %v", recovered),
				},
			}
		}
	}()
	result = d.Handler(ctx, toolCtx, args)
	return result
}

func (r *Registry) Call(ctx context.Context, toolCtx Context, name string, args map[string]any) Result {
	return r.Invoke(ctx, toolCtx, name, args)
}

// Definitions returns all registered tool definitions (schema + handler).
// AgentRunner calls this to build the OpenAI tools list for LLM requests.
func (r *Registry) Definitions() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.defs))
	for name := range r.defs {
		names = append(names, name)
	}
	slices.Sort(names)
	out := make([]Definition, 0, len(names))
	for _, name := range names {
		out = append(out, r.defs[name])
	}
	return out
}

func (r *Registry) ToolDefinitions() []kernel.ToolDefinition {
	defs := r.Definitions()
	out := make([]kernel.ToolDefinition, 0, len(defs))
	for _, def := range defs {
		out = append(out, def.Schema)
	}
	return out
}
