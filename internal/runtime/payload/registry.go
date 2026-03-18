package payload

import "github.com/similarityyoung/simiclaw/pkg/model"

type Registry struct {
	handlers map[string]Handler
}

func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]Handler)}
}

func (r *Registry) Register(handler Handler) {
	if r == nil || handler == nil {
		return
	}
	r.handlers[handler.PayloadType()] = handler
}

func (r *Registry) Resolve(payloadType string) Plan {
	if r != nil {
		if handler, ok := r.handlers[payloadType]; ok {
			return clonePlan(handler.Plan())
		}
	}
	return clonePlan(interactivePlan())
}

func RegisterBuiltins(r *Registry) {
	if r == nil {
		return
	}
	r.Register(messageHandler{})
	r.Register(memoryFlushHandler{})
	r.Register(compactionHandler{})
	r.Register(cronFireHandler{})
}

func interactivePlan() Plan {
	return Plan{
		RunMode:               model.RunModeNormal,
		Kind:                  ExecutionKindInteractive,
		SuppressOutput:        false,
		SuppressStream:        false,
		UserVisible:           true,
		ToolVisible:           true,
		FinalAssistantVisible: true,
	}
}

func clonePlan(plan Plan) Plan {
	plan.AllowedTools = cloneStringSet(plan.AllowedTools)
	plan.MessageMeta = cloneAnyMap(plan.MessageMeta)
	return plan
}

func cloneStringSet(in map[string]struct{}) map[string]struct{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(in))
	for key := range in {
		out[key] = struct{}{}
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
