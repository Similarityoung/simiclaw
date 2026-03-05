# AGENTS.md — SimiClaw Codebase Guide

Single-binary Go agent runtime: `Gateway → EventLoop → StoreLoop → Outbound`.  
Module: `github.com/similarityyoung/simiclaw` | Go 1.25 | Current stage: see `VERSION_STAGE`

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

# Integration tests (requires -tags=integration)
make test-integration

# E2E smoke tests (scope driven by VERSION_STAGE file)
make test-e2e-smoke

# All E2E tests, no cache
make test-e2e

# Acceptance for current milestone
make accept-current

# Doc consistency check
make docs-consistency
```

### Running a Single Test

```bash
# Unit test — any package
go test ./pkg/sessionkey/... -run TestComputeSessionKey_DM_ThreadIgnored -v

# Unit test — specific file's package
go test ./pkg/config/... -run TestLoad -v

# Integration test (must pass build tag)
go test ./tests/integration/... -tags=integration -run TestIngestLifecycleAndCommitOrder -v

# E2E test
go test ./tests/e2e/... -run SmokeM1 -v -count=1
```

### Build / Run

```bash
# Init workspace
go run ./cmd/simiclaw init --workspace ./workspace

# Start server
go run ./cmd/simiclaw serve --workspace ./workspace --listen :8080

# Chat CLI
go run ./cmd/simiclaw chat --workspace ./workspace
```

---

## Project Layout

```
cmd/simiclaw/           CLI entry point; subcommands: init, serve/gateway, chat, version
  internal/             CLI-internal packages (chat, gateway, initcmd, version, common)
pkg/
  model/                Shared types ONLY — no logic. All cross-package types live here.
  config/               Config struct, Default(), Load() from JSON
  logging/              Thin zap wrapper; use logging.L("module")
  gateway/              HTTP handler + Service; ingest, validate, ratelimit, response
  runtime/              EventLoop, EventRepo, RunRepo
  store/                StoreLoop (single writer), SessionStore, workspace fs helpers
  runner/               Runner interface + ProcessRunner
  bus/                  In-process MessageBus
  tools/                Tool registry, tool types, built-in tools (memory_search, memory_get)
  memory/               Memory read (search, get) and write (daily, curated) helpers
  idempotency/          In-memory + file idempotency store
  sessionkey/           Session key computation (SHA-256 deterministic)
  channels/             Channel adapters (Telegram, etc.)
  outbound/             Outbound send hub
  routing/              Session routing helpers
  api/                  App wiring (NewApp, Start, Stop)
tests/
  integration/          Build tag: integration. In-process httptest tests.
  e2e/                  End-to-end smoke tests per milestone (SmokeM1, SmokeM2, ...).
```

---

## Code Style

### General

- Standard `gofmt` formatting; run `make fmt` before committing.
- No external test frameworks — use the standard `testing` package exclusively.
- Only one non-stdlib dependency: `go.uber.org/zap`. Keep external deps minimal.

### Imports

Group and order strictly: stdlib → blank line → project packages (`github.com/similarityyoung/simiclaw/...`).

```go
import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/similarityyoung/simiclaw/pkg/logging"
    "github.com/similarityyoung/simiclaw/pkg/model"
)
```

Never dot-import or alias stdlib packages.

### Naming

| Symbol | Convention | Example |
|---|---|---|
| Exported type/func | PascalCase | `EventLoop`, `NewService` |
| Unexported | camelCase | `tenantLimiter`, `handle` |
| Constants (typed) | PascalCase | `EventStatusAccepted`, `RunModeNormal` |
| Error code consts | `ErrorCode` prefix | `ErrorCodeInvalidArgument` |
| JSON struct tags | `snake_case` + `omitempty` for optional | `json:"event_id"`, `json:"thread_id,omitempty"` |
| Log message keys | dot-separated namespaced | `"gateway.ingest.accepted"` |
| Log field keys | `snake_case` | `"event_id"`, `"latency_ms"` |

### Types & Structs

- All cross-package types belong in `pkg/model`. Never define shared types elsewhere.
- Use typed string constants (`type EventStatus string`) for all finite-value string fields.
- Time fields must be `time.Time` (not `string`) in structs; format as `time.RFC3339Nano` only when serializing to API responses.
- Always use `time.Now().UTC()` — never local time.
- `map[string]any` for flexible metadata/details fields; `map[string]string` for string-only maps.

### Interfaces

Keep interfaces minimal — one or two methods. Define interfaces where consumed, not where implemented.

```go
type Runner interface {
    Run(ctx context.Context, event model.InternalEvent, maxToolRounds int) (RunOutput, error)
}
```

### Constructors

Always provide `NewXxx(...)` constructors returning `*Xxx`. Initialize all fields in the constructor; never rely on zero-value magic for dependencies.

```go
func NewService(cfg config.Config, eventBus *bus.MessageBus, ...) *Service {
    return &Service{cfg: cfg, eventBus: eventBus, ...}
}
```

### Error Handling

- Return errors as the last return value: `(T, error)` or `(T, int, *APIError)` for HTTP handlers.
- Static errors: `errors.New(...)`. Formatted: `fmt.Errorf("...: %v", err)`.
- Error type checks: `errors.Is(err, target)` — never string comparison.
- Explicitly discard unavoidable errors: `_ = f()` with a clear reason in context.
- Never use empty `catch` / ignore blocks without `_ =` acknowledgement.
- Service-layer HTTP methods return `*APIError` (with `StatusCode`, `Code`, `Message`, `Details`). Error codes come from `model.ErrorCode*` constants.

```go
// Correct: explicit discard
_ = l.repo.Update(evt.EventID, func(rec *model.EventRecord) { ... })

// Correct: error propagation
if err := s.events.Put(rec); err != nil {
    return model.IngestResponse{}, 0, &APIError{
        StatusCode: http.StatusInternalServerError,
        Code:       model.ErrorCodeInternal,
        Message:    err.Error(),
    }
}
```

### Logging

Use `logging.L("module")` to get a logger. Always attach context fields with `.With()` at the start of a handler.

```go
logger := logging.L("gateway").With(
    logging.String("event_id", evt.EventID),
    logging.String("tenant_id", s.cfg.TenantID),
)
```

Standard log fields to include on every important log line:

| Field | Type | When |
|---|---|---|
| `status` | string | Always |
| `error_code` | string | On failure |
| `latency_ms` | int64 | On terminal log (success or failure) |
| `event_id` | string | Event handlers |
| `session_key` / `session_id` | string | Session-scoped operations |

Log levels: `Debug` (dev trace), `Info` (happy path), `Warn` (expected failure, rate limit, duplicate), `Error` (unexpected failure, data corruption).

### Concurrency

- Use `context.WithCancel` for lifecycle control; pass `ctx` as first arg everywhere.
- `sync.WaitGroup` for goroutine tracking in loop structs.
- `sync.Mutex` for shared mutable state.
- `atomic.Uint64` for lock-free ID sequences.
- All store writes go through `StoreLoop` (single writer) — never write session files directly.

### Testing Patterns

**Unit tests** — standard Go, no tags required:

```go
func TestComputeSessionKey_DMRequiresParticipant(t *testing.T) {
    _, err := ComputeSessionKey("local", model.Conversation{...}, "default")
    if err == nil {
        t.Fatalf("expected error for missing participant_id")
    }
}
```

**Integration tests** — must have build tag and use `t.TempDir()`:

```go
//go:build integration

func newTestApp(t *testing.T, startLoops bool, queueCap int) *api.App {
    t.Helper()
    cfg := config.Default()
    cfg.Workspace = t.TempDir()
    ...
}
```

- Use `httptest.NewRequest` + `httptest.NewRecorder` for in-process HTTP testing.
- Helper functions always call `t.Helper()` as the first line.
- Assertion style: `t.Fatalf("expected X, got %s", actual)` — no third-party assertion libs.
- Prefer table-driven tests for multiple cases.

---

## Architecture Invariants (Don't Break These)

1. **Single writer**: All session/run/event writes must go through `StoreLoop`. Never write `.jsonl` files from concurrent goroutines.
2. **Commit order**: `AppendBatch (sessions/<id>.jsonl)` → `WriteRun (runs/<id>.json)` → `UpdateSessions (sessions.json)`. This is tested and must not change.
3. **Idempotency before enqueue**: Idempotency check and registration happen in Gateway before the event is published to the bus.
4. **NO_REPLY suppression**: Events with `payload.type` in `{memory_flush, compaction, cron_fire}` must set `RunModeNoReply` and suppress outbound delivery.
5. **`pkg/model` is logic-free**: No functions with business logic in `pkg/model`. Types and constants only.
6. **Workspace boundary**: All file paths must be validated to stay within `runtime/` subdirectories. Path traversal (`../`) is a security issue.
7. **UTC everywhere**: All `time.Time` values must be `.UTC()`. No local time.
