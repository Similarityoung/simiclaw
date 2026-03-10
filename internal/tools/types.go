package tools

import (
	"context"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

// Context is passed to every tool handler invocation.
type Context struct {
	Workspace    string
	Scopes       []string
	Conversation model.Conversation
}

// Result is the return value of a tool handler.
type Result struct {
	Disabled bool              `json:"disabled"`
	Output   map[string]any    `json:"output,omitempty"`
	Error    *model.ErrorBlock `json:"error,omitempty"`
}

// Handler is the function signature every tool must implement.
type Handler func(ctx context.Context, toolCtx Context, args map[string]any) Result

// ParameterSchema describes a single parameter for JSON Schema / Function Calling.
type ParameterSchema struct {
	Type        string                     `json:"type"`
	Description string                     `json:"description,omitempty"`
	Enum        []string                   `json:"enum,omitempty"`
	Minimum     *float64                   `json:"minimum,omitempty"`
	Maximum     *float64                   `json:"maximum,omitempty"`
	Properties  map[string]ParameterSchema `json:"properties,omitempty"`
	Required    []string                   `json:"required,omitempty"`
	Items       *ParameterSchema           `json:"items,omitempty"`
}

// Schema describes a tool in OpenAI Function Calling format.
// The Registry stores this alongside the Handler so AgentRunner can
// pass the tool list to the LLM without any extra bookkeeping.
type Schema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  ParameterSchema `json:"parameters"`
}

// Definition bundles a Schema with its executable Handler.
type Definition struct {
	Schema  Schema
	Handler Handler
}
