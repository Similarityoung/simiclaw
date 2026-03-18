package payload

import "github.com/similarityyoung/simiclaw/pkg/model"

type compactionHandler struct{}

func (compactionHandler) PayloadType() string {
	return "compaction"
}

func (compactionHandler) Plan() Plan {
	return Plan{
		RunMode:               model.RunModeNoReply,
		Kind:                  ExecutionKindMemoryWrite,
		SuppressOutput:        true,
		SuppressStream:        true,
		UserVisible:           false,
		ToolVisible:           false,
		FinalAssistantVisible: false,
		MessageMeta:           map[string]any{"payload_type": "compaction"},
		MemoryWriteTarget:     MemoryWriteTargetCurated,
	}
}
