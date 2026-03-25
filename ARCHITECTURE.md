# ARCHITECTURE

## Purpose

本文给出 SimiClaw 的系统地图、关键分层、主数据流和硬约束。它覆盖运行时后端、CLI 与工作区上下文边界；更细的主题继续拆到 `docs/design-docs/` 与 `docs/references/`。

## System Map

- 入口面:
  - `cmd/simiclaw serve`: 启动 HTTP API、runtime host control 与可选 Telegram runtime
  - `cmd/simiclaw chat`: 交互式 TUI，优先走 `POST /v1/chat:stream`
  - `cmd/simiclaw inspect`: 读取 health、sessions、events、runs、trace
  - `web/`: React + Vite 前端，消费同一组 HTTP 接口
- 核心运行链路:
  - `http` / channels 接收并标准化输入
  - `gateway` 校验、binding/routing、限流、幂等与 session 解析
  - `store` 将 event 持久化到 SQLite
  - `runtime.EventLoop` claim event 并驱动 `runner`
  - `runner` 组装 prompt、调用 provider、执行 tools、生成 trace
  - `store` 在结果事务中提交 messages、runs、sessions、events、outbox、jobs
  - `outbound` / `runtime events` / `http stream` 负责提交后的真实发送与流式通知
- 外部依赖:
  - SQLite (`modernc.org/sqlite`)
  - OpenAI-compatible LLM provider (`github.com/openai/openai-go/v3`)
  - Telegram long polling

## Four-Plane Owner Map

| Plane | 责任 | 主要位置 |
| --- | --- | --- |
| Surface | transport、auth、request normalization、serialization、SSE framing、CLI/Web/Telegram consumer behavior | `cmd/simiclaw/internal/`, `internal/http/`, `internal/channels/` |
| Runtime | command ingress、claim/execute/finalize、worker lifecycle、delivery coordination、runtime observe publication | `internal/gateway/`, `internal/runtime/`, `internal/runner/`, `internal/outbound/` |
| Context/State | durable facts、projections/read models、prompt/context assets、workspace safety boundary | `internal/store/`, `internal/query/`, `internal/prompt/`, `internal/memory/`, `internal/workspace/`, `internal/workspacefile/` |
| Capability Plane | provider、tools，以及后续 skills/MCP/router 等可调用能力 | `internal/provider/`, `internal/tools/` |

补充说明:

- `internal/bootstrap/` 和 `internal/config/` 是 composition root / process wiring，不单列为第五个 plane。
- `web/` 是 Surface contract consumer，不是后端 owner plane。
- `pkg/api/`, `pkg/model/`, `pkg/logging/` 继续承担稳定外部契约与共享基础类型，不承载某个 plane 的内部 owner。

## Current-To-Target Module Mapping

| 当前模块 | 目标 owner | 说明 |
| --- | --- | --- |
| `internal/http/ingest`, `internal/http/query`, `internal/http/stream`, `internal/channels/telegram` | Surface | 只保留 transport 与 surface composition；不拥有 durable state transition |
| `cmd/simiclaw/internal/chat`, `inspect`, `client`, `messages`, `root` | Surface | CLI 只消费 command/query/observe/host-control surface，不定义 runtime 事务语义 |
| `internal/gateway` | Runtime | 作为 command ingress boundary，负责校验、session/routing policy、持久化请求进入点 |
| `internal/runtime`, `internal/runner`, `internal/outbound` | Runtime | 负责执行内核、worker host、observe publication、durable delivery coordination |
| `internal/store/*`, `internal/query/*` | Context/State | facts、projections 与 query read-model owner |
| `internal/prompt`, `internal/memory`, `internal/workspace`, `internal/workspacefile` | Context/State | 上下文资产与 workspace 安全边界，不替代 SQLite facts |
| `internal/provider`, `internal/tools` | Capability Plane | 被 Runtime 调用的能力源，不直接推进 durable state |

## Dependency Directions

- Surface 只调用 Runtime command/observe seam 与 Query read-model seam；不得直接 import `internal/store`、`internal/prompt`、`internal/memory`、`internal/workspace*`、`internal/provider`、`internal/tools`。
- Runtime 不反向依赖 Surface adapter；当前最小 guardrail 是 `internal/gateway`、`internal/runtime`、`internal/outbound` 不 import `internal/http`、`internal/channels` 或 CLI command 包。
- Runtime 对 Context/State 与 Capability 的依赖必须通过显式 owner 收口；当前最小形状是 `internal/runner` 负责消费 `prompt/memory/provider/tools`，而不是把这些依赖散回 `gateway/runtime/outbound`。
- Capability Plane 不推进 durable runtime state；Context/State 不反向拥有 transport 或 execution 逻辑。
- Composition root 只负责 wiring 和生命周期，不重新发明跨 plane 的“万能 service”。

## Primary Flows

1. 写入入口
   `internal/http/ingest` 或 Telegram runtime 把输入标准化为统一 ingress；`gateway.Service` 负责校验、scope 解析、session key 计算、payload hash、路由决策与幂等持久化。
2. 事件执行
   `runtime.EventLoop` 从 SQLite 列出 runnable event，claim 成功后创建 started run，在事务外调用 `runner.ProviderRunner`，最后把执行结果一次性提交回 SQLite。
3. Prompt 与工具回合
  `prompt.Builder` 把 `internal/prompt/system/*.md` 中的 system 模板、memory、workspace 上下文、skills 索引和当前 run context 组装成静态前缀；`runner` 再与 provider 和 `tools.Registry` 协作完成多轮 tool calling。
4. 查询与观测
   `query.Service` 负责 events / runs / sessions 的读模型；`internal/http/query` 和 `inspect` 只消费这些查询接口，而不直接暴露 store 内部类型。

## Streaming Chat Ownership

`POST /v1/chat:stream` 当前仍是一个对外组合入口，但 owner 已经按四块拆清：

- command ingest: `internal/http/ingest` + `internal/gateway`
- runtime observe / replay: `internal/runtime/events`
- SSE framing 与 keepalive: `internal/http/stream`
- client-side retry / polling fallback: `cmd/simiclaw/internal/client` 与对应 consumer

## Enforced Boundaries

- `tests/architecture/four_plane_boundaries_test.go`、`tests/architecture/boundaries_test.go` 与 `tests/architecture/runtime_kernel_boundaries_test.go` 会固定 owner map，并阻止 `internal/http` / `internal/channels` 直接依赖 context assets 或 capability plane，也阻止 runtime plane 反向依赖 surface adapters。
- 现有 store guardrail 继续阻止 `internal/http`、`internal/query`、`internal/runner`、`internal/runtime`、`internal/channels`、`internal/workspace` 直接依赖 `internal/store`；gateway 通过自有 contract 与事实层解耦。
- `pkg/api` 是稳定对外契约；内部子系统通过各自的 `internal/<subsystem>/model` 传递局部 DTO。
- 只有 ingest 入口能直接调用 `IngestEvent`；写路径不能在系统里随意旁路。

## Runtime Invariants

1. SQLite 是唯一事实源。
2. 所有写事务都通过单 writer handle。
3. Gateway 必须先持久化 event，再尝试 enqueue。
4. `POST /v1/events:ingest` 必须显式提供 `idempotency_key`。
5. event 只有在 claim 事务成功后才进入 `processing`。
6. EventLoop 必须按“claim -> 事务外执行 -> 结果事务提交”两阶段运行。
7. 真实出站发送只能发生在 outbox 提交之后。
8. `sessions` 只是 derived cache；FTS 只由 SQLite trigger 维护。
9. `memory_flush`、`compaction`、`cron_fire` 必须走 `RunModeNoReply`。
10. workspace runtime 只允许 `memory/`、`runtime/app.db`、`runtime/native/`。

## Related Docs

- 入口导航: [`AGENTS.md`](AGENTS.md)
- 文档首页: [`docs/index.md`](docs/index.md)
- 四块骨架: [`docs/design-docs/four-plane-architecture.md`](docs/design-docs/four-plane-architecture.md)
- 运行链路细节: [`docs/design-docs/runtime-flow.md`](docs/design-docs/runtime-flow.md)
- 模块边界细节: [`docs/design-docs/module-boundaries.md`](docs/design-docs/module-boundaries.md)
- Prompt / Workspace 上下文: [`docs/design-docs/prompt-and-workspace-context.md`](docs/design-docs/prompt-and-workspace-context.md)
- 配置参考: [`docs/references/configuration.md`](docs/references/configuration.md)
