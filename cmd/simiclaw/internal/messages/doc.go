// Package messages contains user-facing text resources.
//
// Current layout:
//   - commands.go: Cobra command descriptions, flag help, and generic CLI errors
//   - chat.go: chat CLI / TUI / REPL strings and formatting helpers
//   - inspect.go: inspect command output labels and formatting helpers
//
// Runtime model prompts stay in internal/prompt.
// Workspace scaffold templates stay in internal/workspace/templates/.
// This package is intentionally CLI-only and lives under cmd/simiclaw/internal.
package messages
