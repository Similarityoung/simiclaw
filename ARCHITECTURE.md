# ARCHITECTURE

## Purpose

本文给出 SimiClaw 的系统地图、关键分层、主数据流和硬约束。它覆盖运行时后端、CLI 与工作区上下文边界；更细的主题继续拆到 `docs/design-docs/` 与 `docs/references/`。

## System Map

- 入口面:
  - `cmd/simiclaw serve`: 启动 HTTP API、runtime supervisor 与可选 Telegram runtime
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
  - `outbound` / `streaming` 负责提交后的真实发送与流式通知
- 外部依赖:
  - SQLite (`modernc.org/sqlite`)
  - OpenAI-compatible LLM provider (`github.com/openai/openai-go/v3`)
  - Telegram long polling

## Layers

| 层 | 责任 | 主要位置 |
| --- | --- | --- |
| 入口层 | CLI、HTTP、前端与渠道适配 | `cmd/simiclaw/`, `internal/http/`, `internal/channels/`, `web/` |
| 应用装配层 | 配置加载、依赖组装、生命周期管理 | `internal/bootstrap/`, `internal/config/` |
| 写路径与执行层 | gateway、event loop、runner、worker、outbox | `internal/gateway/`, `internal/runtime/`, `internal/runner/`, `internal/outbound/` |
| 持久化与查询层 | SQLite schema、事务、读服务 | `internal/store/`, `internal/query/`, `internal/readmodel/` |
| 上下文与能力层 | prompt、memory、skills、workspace 文件边界 | `internal/prompt/`, `internal/memory/`, `internal/tools/`, `internal/workspace/`, `internal/workspacefile/` |
| 对外稳定契约 | HTTP / SSE / CLI wire model 与共享类型 | `pkg/api/`, `pkg/model/`, `pkg/logging/` |

## Primary Flows

1. 写入入口
   `internal/http/ingest` 或 Telegram runtime 把输入标准化为统一 ingress；`gateway.Service` 负责校验、scope 解析、session key 计算、payload hash、路由决策与幂等持久化。
2. 事件执行
   `runtime.EventLoop` 从 SQLite 列出 runnable event，claim 成功后创建 started run，在事务外调用 `runner.ProviderRunner`，最后把执行结果一次性提交回 SQLite。
3. Prompt 与工具回合
   `prompt.Builder` 把 system prompt、memory、workspace 上下文、skills 索引和当前 run context 组装成静态前缀；`runner` 再与 provider 和 `tools.Registry` 协作完成多轮 tool calling。
4. 查询与观测
   `query.Service` 负责 events / runs / sessions 的读模型；`internal/http/query` 和 `inspect` 只消费这些查询接口，而不直接暴露 store 内部类型。

## Enforced Boundaries

- `tests/architecture/boundaries_test.go` 会阻止 `internal/http`、`internal/ingest/port` 以外的上层入口、`internal/query`、`internal/runner`、`internal/runtime`、`internal/channels`、`internal/workspace` 直接依赖 `internal/store`。
- `internal/readmodel` 只能被 `internal/store` 使用。
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
- 运行链路细节: [`docs/design-docs/runtime-flow.md`](docs/design-docs/runtime-flow.md)
- 模块边界细节: [`docs/design-docs/module-boundaries.md`](docs/design-docs/module-boundaries.md)
- Prompt / Workspace 上下文: [`docs/design-docs/prompt-and-workspace-context.md`](docs/design-docs/prompt-and-workspace-context.md)
- 配置参考: [`docs/references/configuration.md`](docs/references/configuration.md)
