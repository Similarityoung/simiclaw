package payload

import "github.com/similarityyoung/simiclaw/pkg/model"

var cronFireAllowedTools = map[string]struct{}{
	"memory_search": {},
	"memory_get":    {},
	"context_get":   {},
}

type cronFireHandler struct{}

func (cronFireHandler) PayloadType() string {
	return "cron_fire"
}

func (cronFireHandler) Plan() Plan {
	return Plan{
		RunMode:               model.RunModeNoReply,
		Kind:                  ExecutionKindSuppressedLLM,
		SuppressOutput:        true,
		SuppressStream:        true,
		UserVisible:           false,
		ToolVisible:           false,
		FinalAssistantVisible: false,
		AllowedTools:          cronFireAllowedTools,
		MessageMeta:           map[string]any{"payload_type": "cron_fire"},
	}
}
