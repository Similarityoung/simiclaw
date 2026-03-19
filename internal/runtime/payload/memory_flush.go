package payload

import "github.com/similarityyoung/simiclaw/pkg/model"

type memoryFlushHandler struct{}

func (memoryFlushHandler) PayloadType() string {
	return "memory_flush"
}

func (memoryFlushHandler) Plan() Plan {
	return Plan{
		RunMode:               model.RunModeNoReply,
		Kind:                  ExecutionKindMemoryWrite,
		SuppressOutput:        true,
		SuppressStream:        true,
		UserVisible:           false,
		ToolVisible:           false,
		FinalAssistantVisible: false,
		MessageMeta:           map[string]any{"payload_type": "memory_flush"},
		MemoryWriteTarget:     MemoryWriteTargetDaily,
	}
}
