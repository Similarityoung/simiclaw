# Implementation Plan: Google ADK Refactoring for SimiClaw

## Goal
Completely refactor SimiClaw to integrate Google ADK (`adk-go`), adding dynamic skill and tool (function call) support similar to OpenClaw. Allow plugin-like dynamic loading of tools/skills via MCP and Sub-Agents.

## Key Decisions
1. **Total ADK Adoption**: The existing `EventLoop`, `StoreLoop`, and `ProcessRunner` will be entirely replaced by ADK's native `Runner`, `SessionService`, and `MemoryService`.
2. **Tools (Function Calling)**: Built-in tools (`file_read`, `file_write`, `file_edit`, `bash`) implemented statically using ADK's `functiontool.New(...)`.
3. **Skills Architecture**: True OpenClaw style. Skills are NOT sub-agents. They are dynamic Context Injections. We will build a Context Assembler that reads `workspace/skills/**/SKILL.md` and injects them into the LLM's system prompt or context window. The LLM executes these skills using the `bash` tool.
4. **Plugins (Extensions)**: Real MCP server integrations loaded via ADK's `mcptoolset.New(...)` for structured third-party API access.
4. **Git Strategy**: Create a new branch `adk-refactor` and commit iteratively for each small feature.

## Guardrails & Security
1. **Path Sandbox**: All file operations MUST be restricted to the `workspace/` directory to prevent path traversal escapes.
2. **Bash Sandbox**: Bash tool should have a timeout (e.g., 60s) and explicitly capture stderr/stdout.
3. **Skill Security**: Skills (bash execution) should run under strict timeouts and potentially directory boundaries.
4. **Tool Safety**: Ensure tools gracefully handle errors and return them to the LLM rather than crashing the process.

## Tasks

### Phase 1: Foundation & Setup
- [ ] 1. Create a new branch `adk-refactor` (`git checkout -b adk-refactor`).
- [ ] 2. Update `go.mod` to add Google ADK dependency (`go get google.golang.org/adk` or fallback to SDK if available).
- [ ] 3. Create `pkg/adkruntime` directory. We will use a **Parallel Dual-Stack Strategy**: keep existing `pkg/runtime`, `pkg/runner`, `pkg/store` intact while building the new ADK stack to ensure we can test and compare. Commit setup.

### Phase 2: Core ADK Integration (Parallel Stack)
- [ ] 4. Initialize ADK's `Runner`, `SessionService`, and `MemoryService` in `pkg/adkruntime`.
- [ ] 5. Define the primary `LlmAgent` in `pkg/adkruntime` with a system prompt fitting SimiClaw.
- [ ] 6. Integrate `pkg/adkruntime` into `Gateway` behind a feature flag (or specific test endpoints) to allow side-by-side testing of HTTP events routed into ADK sessions. Commit.

### Phase 3: Built-in Tools implementation
- [ ] 7. Create `pkg/tools/builtin.go`. Implement `file_read` tool using ADK's `functiontool.New`. Ensure path is constrained to workspace.
- [ ] 8. Implement `file_write` and `file_edit` tools. Ensure path constraint. Add unit tests for safety.
- [ ] 9. Implement `bash` tool with a strict timeout and execution directory constraint. Add unit tests.
- [ ] 10. Register these built-in tools to the primary `LlmAgent`. Commit.

### Phase 4: Core Validation & HTTP Contract
- [ ] 11. Refactor `cmd/simiclaw chat` to support connecting to the new ADK feature flag.
- [ ] 12. Run contract tests. Ensure that the core loop is functional, NO_REPLY works, and idempotency works in the new stack. Commit.

### Phase 5: Dynamic Skills (Context Injection) & Plugins (MCP)
- [ ] 13. Create `pkg/skills/assembler.go`. Implement Context Injection: before a run, scan `workspace/skills/`, read `SKILL.md` files, and append their contents to the ADK `LlmAgent` prompt/system instruction.
- [ ] 14. Ensure the `bash` tool (implemented in Phase 3) works seamlessly with these injected instructions, proving the "OpenClaw skill evolution loop" is viable.
- [ ] 15. Create `pkg/plugins/dynamic.go`. Implement plugin loading via `mcptoolset.New` reading configuration from a `plugins.json`. Ensure an allowlist/trust model is in place for MCP servers. Commit.

### Phase 6: Cutover & Cleanup
- [ ] 16. Remove the feature flags and fully point all Gateway traffic to the new ADK-based runtime.
- [ ] 17. Delete the old `pkg/runtime`, `pkg/runner`, and `pkg/store` packages. Remove dead code.
- [ ] 18. Refactor existing tests and add new tests for the ADK flow. Ensure `make test-unit` and `make test-integration` pass.
- [ ] 19. Final commit and push.
## Final Verification Wave
1. **Contract Testing**: Ensure HTTP API inputs/outputs remain compatible or explicitly documented if changed.
2. **Safety Checks**: Run tests proving `bash` times out and `file_write` prevents path traversal.
3. **Run existing test suites**: `make test-unit`, `make test-integration`, `make accept-current`.