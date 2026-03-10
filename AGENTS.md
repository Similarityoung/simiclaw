# AGENTS.md — SimiClaw Codebase Guide

Single-binary Go agent runtime built around a SQLite-first state machine.
Module: `github.com/similarityyoung/simiclaw` | Go 1.25 | Current stage: `V1`

---

## Build & Commands

```bash
# Format all Go code
make fmt

# Static analysis
make vet

# Lint (golangci-lint if installed, falls back to go vet)
make lint

# Unit tests with coverage
make test-unit

# Race-detection tests on core packages only
make test-unit-race-core

# Integration tests
make test-integration

# E2E smoke tests
make test-e2e-smoke

# All E2E tests, no cache
make test-e2e

# Acceptance for v1
make accept-v1

# Acceptance for current stage
make accept-current
```

### Running a Single Test

```bash
go test ./internal/session/... -run TestComputeKeyDMThreadIgnored -v
go test ./internal/config/... -run TestLoad -v
go test ./tests/integration/... -tags=integration -run TestRuntimeSQLiteLifecycle -v
go test ./tests/e2e/... -run SmokeV1 -v -count=1
```

### Build / Run

```bash
# Init workspace
go run ./cmd/simiclaw init --workspace ./workspace

# Start server
go run ./cmd/simiclaw serve --workspace ./workspace --listen :8080

# Chat CLI
go run ./cmd/simiclaw chat --base-url http://127.0.0.1:8080
```

### Real LLM Example

```bash
cat > .env <<'EOF'
OPENAI_API_KEY=your-api-key
OPENAI_BASE_URL=https://api.deepseek.com
LLM_MODEL=openai/deepseek-chat
EOF
```

Legacy aliases `LLM_API_KEY` / `LLM_BASE_URL` are also accepted.

---

## Project Layout

```text
cmd/simiclaw/           CLI entry point; subcommands: init, serve/gateway, chat, inspect, version, completion
  internal/             CLI-internal packages (chat, gateway, initcmd, inspect, version, common)
pkg/
  logging/              Thin zap wrapper
  model/                Shared cross-package types only
internal/
  bootstrap/            App wiring, dependency assembly, process lifecycle
  channels/             CLI / Telegram adapters
  config/               Config struct, defaults, env/file loading
  gateway/              HTTP ingest validation, rate limit, responses
  httpapi/              HTTP routes, handlers, auth, pagination
  memory/               Workspace memory read/write helpers
  outbound/             Outbound sender interface
  prompt/               Prompt builder orchestration
  provider/             LLMProvider abstraction, fake provider, OpenAI-compatible provider
  query/                Read-side query service for events / runs / sessions
  runner/               Provider-driven runtime execution
  runtime/              EventLoop, workers, supervisor lifecycle
  session/              Session key computation
  store/                SQLite bootstrapping, schema, queries, repo
  systemprompt/         Embedded runtime system prompt fragments
  tools/                Tools / skills execution surface
tests/
  integration/          In-process integration tests
  e2e/                  End-to-end smoke tests
```

---

## Code Style

- Standard `gofmt` formatting; run `make fmt` before committing.
- Use the standard `testing` package only.
- Keep imports ordered as stdlib first, then project packages.
- Shared cross-package types belong in `pkg/model`.
- Use `time.Now().UTC()` everywhere.
- Use `map[string]any` for flexible payload/details fields.
- Keep interfaces minimal and define them where consumed.

---

## Runtime Invariants

1. SQLite is the only source of truth.
2. All write transactions go through the single writer handle.
3. Gateway must persist the event before enqueueing it.
4. `POST /v1/events:ingest` requires an explicit `idempotency_key`.
5. An event reaches `processing` only after a successful claim transaction.
6. The EventLoop uses two-phase processing:
   1. claim event + create `runs(started)`
   2. execute LLM/tools outside the write transaction
   3. commit messages/runs/sessions/events/outbox/jobs in one result transaction
7. Real outbound sending must happen after outbox persistence commits.
8. `sessions` is a derived cache, not the fact source.
9. FTS is maintained only by SQLite triggers.
10. `payload.type in {memory_flush, compaction, cron_fire}` must use `RunModeNoReply`.

---

## Workspace Layout

Only these runtime paths should be created under a workspace:

```text
workspace/
  memory/
  runtime/
    app.db
    native/
```

Legacy file-runtime traces such as `sessions.json`, `runtime/approvals`, `runtime/outbound_spool`, and `workspace/evolution` must be rejected or removed during migration to the SQLite runtime.
