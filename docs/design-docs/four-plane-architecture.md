# Four-Plane Architecture

## Summary

`spec03` 把 SimiClaw 的后端 owner 固定为四块：`Surface`、`Runtime`、`Context/State`、`Capability Plane`。这不是目录搬家说明，而是“谁拥有什么职责、允许依赖谁、不能回流什么混合逻辑”的长期文档。

## Owner Map

| Plane | 必须拥有 | 不能拥有 | 代表模块 |
| --- | --- | --- | --- |
| Surface | transport、auth、request normalization、serialization、SSE framing、CLI/Web/Telegram consumer behavior | durable state transition、provider 选择、workspace 直接访问 | `cmd/simiclaw/internal/*`, `internal/http/*`, `internal/channels/*` |
| Runtime | command ingress、claim/execute/finalize、worker lifecycle、observe publication、delivery coordination | HTTP/SSE/CLI/Telegram protocol formatting、workspace ad hoc access | `internal/gateway/*`, `internal/runtime/*`, `internal/runner/*`, `internal/outbound/*` |
| Context/State | SQLite facts、projections/read models、prompt/context assets、workspace safety boundary | transport 逻辑、runtime orchestration、provider/tool invocation | `internal/store/*`, `internal/query/*`, `internal/prompt`, `internal/memory`, `internal/workspace*` |
| Capability Plane | model provider、tools，以及后续 skills/MCP/router 等能力源 | durable state mutation、transport 行为 | `internal/provider/*`, `internal/tools/*` |

补充:

- `internal/bootstrap/`、`internal/config/` 是 composition root，不额外形成第五块。
- `web/` 是 Surface contract consumer，不是后端 adapter owner。
- `pkg/api`、`pkg/model`、`pkg/logging` 继续是共享契约与基础类型，不作为某一块的内部 owner。

## Dependency Directions

| From | To | 规则 |
| --- | --- | --- |
| Surface | Runtime / Query | 允许；Surface 只消费 command/query/observe seam |
| Surface | `internal/store` / `internal/prompt` / `internal/memory` / `internal/workspace*` / Capability Plane | 禁止 |
| Runtime | Context/State | 允许，但必须通过显式 owner 收口；当前以 `internal/runner` 与 `internal/runner/context` 为主 |
| Runtime | Capability Plane | 允许，但必须通过 provider/tool contracts 调用 |
| Runtime | Surface | 禁止 |
| Capability Plane | durable facts / transport | 禁止 |

当前已落地的最小 guardrail 见：

- `tests/architecture/four_plane_boundaries_test.go`
- `tests/architecture/boundaries_test.go`
- `tests/architecture/runtime_kernel_boundaries_test.go`

## Current-To-Target Mapping

| 当前模块 | 目标 owner | 当前阶段解释 |
| --- | --- | --- |
| `internal/http/ingest` | Surface | command ingress 的 HTTP transport 壳层 |
| `internal/http/query` | Surface | query read-model 的 HTTP serialization 壳层 |
| `internal/http/stream` | Surface | `chat:stream` 的 SSE framing 和 observe subscription adapter |
| `internal/channels/telegram` | Surface | Telegram transport normalization 与 ingress adapter |
| `cmd/simiclaw/internal/chat`, `inspect`, `client`, `messages`, `root` | Surface | CLI surface 与 consumer-side fallback |
| `internal/gateway` | Runtime | command ingress boundary、session/routing policy、幂等写入口 |
| `internal/runtime/*` | Runtime | kernel、workers、observe publication、host control |
| `internal/runner/*` | Runtime | agent execution、prompt glue、tool loop、trace assembly |
| `internal/outbound/*` | Runtime | durable delivery coordination |
| `internal/store/*` | Context/State | facts / projections / low-level queries |
| `internal/query/*` | Context/State | query service 和 query DTO |
| `internal/prompt`, `internal/memory`, `internal/workspace`, `internal/workspacefile` | Context/State | context assets 与 workspace safety |
| `internal/provider/*`, `internal/tools/*` | Capability Plane | 可调用能力源 |

## `chat:stream` Owner Split

当前组合入口仍保留原有对外契约，但内部 owner 要按下面理解：

- command ingest: `internal/http/ingest` + `internal/gateway`
- runtime observe / replay: `internal/runtime/events`
- server-side stream framing: `internal/http/stream`
- client retry / polling fallback: `cmd/simiclaw/internal/client`

US1 阶段先把 owner 说明和 guardrail 固定下来；真正把混合实现进一步拆薄，属于后续 US2 / US3。

## Composition Root

`internal/bootstrap/app.go` 只负责把 Surface、Runtime、Context/State、Capability Plane 接起来，并托管生命周期。它可以 import 多个 plane，但不能演化成新的业务 owner。

`cmd/simiclaw/internal/root/command.go` 只负责 CLI surface tree 和全局 flags 的拼装；具体 wiring 与 runtime 启停仍下沉到子命令或 bootstrap。

## Verification

- `go test ./tests/architecture/... -v`
- `make docs-style`

通过这两个最小 gate 后，维护者应能从测试和文档中回答“这个模块属于哪一块、允许依赖谁、不能拥有谁”。
