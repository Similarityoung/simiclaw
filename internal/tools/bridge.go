package tools

import pkgtools "github.com/similarityyoung/simiclaw/pkg/tools"

type Context = pkgtools.Context
type Result = pkgtools.Result
type Handler = pkgtools.Handler
type ParameterSchema = pkgtools.ParameterSchema
type Schema = pkgtools.Schema
type Definition = pkgtools.Definition
type Registry = pkgtools.Registry
type WebSearchOptions = pkgtools.WebSearchOptions
type WebFetchOptions = pkgtools.WebFetchOptions

func NewRegistry() *Registry { return pkgtools.NewRegistry() }

func RegisterMemorySearch(reg *Registry) { pkgtools.RegisterMemorySearch(reg) }

func RegisterMemoryGet(reg *Registry) { pkgtools.RegisterMemoryGet(reg) }

func RegisterContextGet(reg *Registry) { pkgtools.RegisterContextGet(reg) }

func RegisterWorkspacePatch(reg *Registry) { pkgtools.RegisterWorkspacePatch(reg) }

func RegisterWorkspaceDelete(reg *Registry) { pkgtools.RegisterWorkspaceDelete(reg) }

func RegisterBuiltins(reg *Registry) { pkgtools.RegisterBuiltins(reg) }

func RegisterWebSearch(reg *Registry, opts WebSearchOptions) { pkgtools.RegisterWebSearch(reg, opts) }

func RegisterWebFetch(reg *Registry, opts WebFetchOptions) { pkgtools.RegisterWebFetch(reg, opts) }
