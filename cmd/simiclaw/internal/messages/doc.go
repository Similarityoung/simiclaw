// Package messages contains Surface-plane CLI text resources.
//
// Current layout:
//   - commands.go: Cobra command descriptions, flag help, and generic CLI errors
//   - chat.go: chat CLI / TUI / REPL strings and formatting helpers
//   - inspect.go: inspect command output labels and formatting helpers
//
// Runtime prompts stay in internal/prompt.
// Workspace scaffold templates stay in internal/workspace/templates/.
// This package is intentionally CLI-only and must not become a backend runtime
// owner or a shared wire-contract bucket.
package messages
