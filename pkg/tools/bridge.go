package tools

import internaltools "github.com/similarityyoung/simiclaw/internal/tools"

type Context = internaltools.Context
type Result = internaltools.Result
type Handler = internaltools.Handler
type ParameterSchema = internaltools.ParameterSchema
type Schema = internaltools.Schema
type Definition = internaltools.Definition
type Registry = internaltools.Registry
type WebSearchOptions = internaltools.WebSearchOptions
type WebFetchOptions = internaltools.WebFetchOptions

func NewRegistry() *Registry { return internaltools.NewRegistry() }

func RegisterMemorySearch(reg *Registry) { internaltools.RegisterMemorySearch(reg) }

func RegisterMemoryGet(reg *Registry) { internaltools.RegisterMemoryGet(reg) }

func RegisterContextGet(reg *Registry) { internaltools.RegisterContextGet(reg) }

func RegisterWorkspacePatch(reg *Registry) { internaltools.RegisterWorkspacePatch(reg) }

func RegisterWorkspaceDelete(reg *Registry) { internaltools.RegisterWorkspaceDelete(reg) }

func RegisterBuiltins(reg *Registry) { internaltools.RegisterBuiltins(reg) }

func RegisterWebSearch(reg *Registry, opts WebSearchOptions) {
	internaltools.RegisterWebSearch(reg, opts)
}

func RegisterWebFetch(reg *Registry, opts WebFetchOptions) { internaltools.RegisterWebFetch(reg, opts) }
