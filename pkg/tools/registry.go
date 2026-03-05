package tools

import (
	"context"
	"sync"

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

func (r *Registry) Call(ctx context.Context, toolCtx Context, name string, args map[string]any) Result {
	r.mu.RLock()
	d, ok := r.defs[name]
	r.mu.RUnlock()
	if !ok {
		return Result{Error: &model.ErrorBlock{Code: model.ErrorCodeInvalidArgument, Message: "tool not found"}}
	}
	return d.Handler(ctx, toolCtx, args)
}

// Definitions returns all registered tool definitions (schema + handler).
// AgentRunner calls this to build the OpenAI tools list for LLM requests.
func (r *Registry) Definitions() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.defs))
	for _, d := range r.defs {
		out = append(out, d)
	}
	return out
}
