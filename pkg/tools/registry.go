package tools

import (
	"context"
	"sync"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type Registry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

func NewRegistry() *Registry {
	return &Registry{handlers: map[string]Handler{}}
}

func (r *Registry) Register(name string, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[name] = handler
}

func (r *Registry) Call(ctx context.Context, toolCtx Context, name string, args map[string]any) Result {
	r.mu.RLock()
	h, ok := r.handlers[name]
	r.mu.RUnlock()
	if !ok {
		return Result{Error: &model.ErrorBlock{Code: model.ErrorCodeInvalidArgument, Message: "tool not found"}}
	}
	return h(ctx, toolCtx, args)
}
