package payload

import "github.com/similarityyoung/simiclaw/pkg/model"

type ExecutionKind string

const (
	ExecutionKindInteractive   ExecutionKind = "interactive"
	ExecutionKindMemoryWrite   ExecutionKind = "memory_write"
	ExecutionKindSuppressedLLM ExecutionKind = "suppressed_llm"
)

type MemoryWriteTarget string

const (
	MemoryWriteTargetNone    MemoryWriteTarget = ""
	MemoryWriteTargetDaily   MemoryWriteTarget = "daily"
	MemoryWriteTargetCurated MemoryWriteTarget = "curated"
)

type Plan struct {
	RunMode               model.RunMode
	Kind                  ExecutionKind
	SuppressOutput        bool
	SuppressStream        bool
	UserVisible           bool
	ToolVisible           bool
	FinalAssistantVisible bool
	AllowedTools          map[string]struct{}
	MessageMeta           map[string]any
	MemoryWriteTarget     MemoryWriteTarget
}

type Handler interface {
	PayloadType() string
	Plan() Plan
}
