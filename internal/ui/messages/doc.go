// Package messages contains user-facing text resources.
//
// Current layout:
//   - commands.go: Cobra command descriptions, flag help, and generic CLI errors
//   - chat.go: chat CLI / TUI / REPL strings and formatting helpers
//   - inspect.go: inspect command output labels and formatting helpers
//
// Runtime model prompts stay in internal/systemprompt.
// Workspace scaffold templates stay in internal/workspace/templates/.
// Future Web or Telegram surfaces can add channel-specific files here without mixing
// user-visible messages into runtime prompt text.
package messages
