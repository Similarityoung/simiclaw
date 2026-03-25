package tools

import (
	"context"

	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
)

type Context = kernel.ToolContext
type Result = kernel.ToolResult

// Handler is the function signature every tool must implement.
type Handler func(ctx context.Context, toolCtx Context, args map[string]any) Result

type ParameterSchema = kernel.ParameterSchema
type Schema = kernel.ToolDefinition

// Definition bundles a Schema with its executable Handler.
type Definition struct {
	Schema  Schema
	Handler Handler
}
