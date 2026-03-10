# CLAUDE.md
This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository overview

- This repository is a single-host, single-process Go Agent Runtime.
- The runtime is SQLite-first. `workspace/runtime/app.db` is the single source of truth; session views are derived from SQLite state rather than an independent source of truth.
- Workspace filesystem state is intentionally small. The persistent paths that matter are:
  - `workspace/memory/**`
  - `workspace/runtime/native/**`
  - `workspace/runtime/app.db`
- `chat` is a client command that talks to the HTTP service. The primary interactive endpoint is the streaming chat endpoint `POST /v1/chat:stream`.
- Built-in tools are injected into the runner through the tools registry.
- Runtime execution is driven jointly by the event loop and supervisor workers.

## Common commands

```bash
# Initialize a workspace
go run ./cmd/simiclaw init --workspace ./workspace

# Start the HTTP runtime
go run ./cmd/simiclaw serve --workspace ./workspace --listen :8080

# Start the chat CLI (talks to the HTTP service)
go run ./cmd/simiclaw chat

# Formatting / checks
make fmt
make vet
make lint

# Tests
make test-unit
make test-unit-race-core
make test-integration
make test-e2e-smoke

# Acceptance entry points
make accept-v1
make accept-current

# Run a single unit-style test
go test ./internal/... -run TestName

# Run a single integration test
go test ./tests/integration/... -tags=integration -run TestName
```

## Architecture map

- `cmd/simiclaw/main.go`: CLI entrypoint. Dispatches `init | serve | gateway | chat | inspect | version | completion`.
- `internal/bootstrap/app.go`: application assembly. Wires DB, tools registry, provider factory, stream hub, runner, event loop, supervisor, gateway service, and HTTP server handler.
- `internal/gateway/service.go` + `internal/ingest/service.go`: ingest boundary. HTTP and channel adapters validate transport requests, then delegate idempotency, scope resolution, persistence, and enqueue orchestration to the ingest application service.
- `internal/httpapi/server.go` + `internal/httpapi/routes.go`: HTTP exposure layer for `healthz`, `readyz`, events, runs, sessions, ingest, and streaming chat.
- `internal/query/service.go`: read-side query boundary for paginated events, runs, and sessions listings.
- `internal/runtime/eventloop.go`: core execution loop. Claims runnable events, invokes the runner, finalizes runs, writes messages / trace / outbox state, and publishes terminal stream events.
- `internal/runtime/workers.go`: supervisor-managed background workers for heartbeat, processing recovery, outbox retry, delayed jobs, and cron.
- `internal/runner/runner.go`: loads recent message history and SQLite FTS hits, resolves the configured LLM provider, executes tools through the registry, and produces trace/output. `memory_flush`, `cron_fire`, and `compaction` are handled as no-reply payloads that write directly into workspace memory.
- `internal/store/db.go` + `internal/store/history.go`: workspace initialization, SQLite open/schema validation, read/write connections, recent history reads, and FTS-backed message search.
- `internal/config/config.go`: defaults, file/env overrides, and provider selection/validation.
- `internal/systemprompt/system.go`: embedded runtime system prompt fragments used by the prompt builder.
- `internal/tools/registry.go`, `internal/tools/memory_search.go`, `internal/tools/memory_get.go`, `internal/tools/web_search.go`, `internal/tools/web_fetch.go`: built-in tool registration and execution surface exposed to the runner.

## Suggested reading paths

- To understand how a request flows from ingest to execution:
  - `internal/gateway/service.go`
  - `internal/runtime/eventloop.go`
  - `internal/runner/runner.go`
- To understand startup and wiring:
  - `cmd/simiclaw/main.go`
  - `internal/bootstrap/app.go`
- To understand persistence and retrieval:
  - `internal/store/db.go`
  - `internal/store/history.go`

## Validation entry points

- When changing the core runtime path, start by checking:
  - `tests/integration/runtime_integration_test.go`
  - `tests/e2e/smoke_v1_test.go`
- Repository test entry points:
  - unit: `make test-unit`
  - race on core packages: `make test-unit-race-core`
  - integration: `make test-integration` or `go test ./tests/integration/... -tags=integration`
  - e2e smoke: `make test-e2e-smoke` or `go test ./tests/e2e/... -run 'SmokeV1'`
  - acceptance: `make accept-v1` and `make accept-current`
