# AGENTS.md — SimiClaw 导航图

SimiClaw 是一个围绕 SQLite-first 状态机构建的单二进制 Go agent runtime。这个文件只保留高杠杆导航；实现细节、设计说明和执行记录统一收敛到 [`ARCHITECTURE.md`](ARCHITECTURE.md) 与 [`docs/`](docs/index.md)。

`Module`: `github.com/similarityyoung/simiclaw` | `Go`: `1.25` | `Stage`: `V1`

## Quick Start

- 格式化代码: `make fmt`
- 单元测试: `make test-unit`
- 当前阶段验收: `make accept-current`
- 初始化工作区: `go run ./cmd/simiclaw init --workspace ./workspace`
- 启动服务: `go run ./cmd/simiclaw serve --workspace ./workspace --listen :8080`
- 启动交互式 CLI: `go run ./cmd/simiclaw chat --base-url http://127.0.0.1:8080`

真实模型的最小 `.env` 示例:

```bash
OPENAI_API_KEY=your-api-key
OPENAI_BASE_URL=https://api.deepseek.com
LLM_MODEL=openai/deepseek-chat
```

兼容旧别名 `LLM_API_KEY` / `LLM_BASE_URL`。

## Repo Map

- `cmd/simiclaw/`: CLI 入口与子命令，含 `serve`, `init`, `chat`, `inspect`, `version`, `completion`
- `internal/bootstrap/`: 应用装配与进程生命周期
- `internal/gateway/`: 写路径入口、binding/routing、校验、限流、幂等与 session scope 解析
- `internal/runtime/` + `internal/runner/`: EventLoop、kernel、payload/worker/lane owner、LLM 执行、上下文组装、工具回合与最终提交
- `internal/store/`: SQLite 初始化、schema，以及 `tx` / `projections` / `queries` 三层事实与读写职责
- `internal/query/` + `internal/http/`: 查询模型、HTTP 读接口与 runtime event stream
- `internal/prompt/` + `internal/memory/` + `internal/tools/`: prompt 组装、memory、tool surface
- `internal/workspace/` + `internal/workspacefile/`: 工作区脚手架与安全文件边界
- `pkg/`: 对外稳定契约，尤其是 `pkg/api`
- `web/`: React + Vite 前端
- `tests/`: `architecture`, `integration`, `e2e`
- `docs/`: 文档系统入口、设计索引、参考资料、执行计划、质量评分

## Must-Know Invariants

1. SQLite 是唯一事实源；`sessions` 只是派生缓存。
2. 所有写事务必须经过单 writer；真实出站发送只能发生在 outbox 持久化提交之后。
3. Gateway 必须先持久化 event，再尝试入队；`POST /v1/events:ingest` 必须显式提供 `idempotency_key`。
4. EventLoop 采用两阶段处理：先 claim event 并创建 `runs(started)`，再在事务外执行 LLM / tools，最后一次性提交 messages、runs、sessions、events、outbox、jobs。
5. event 只有在 claim 成功后才能进入 `processing`。
6. `payload.type in {memory_flush, compaction, cron_fire}` 必须走 `RunModeNoReply`。
7. FTS 仅由 SQLite trigger 维护，不在应用层手工同步。
8. workspace 运行时只允许 `memory/`、`runtime/app.db` 和 `runtime/native/`；旧文件式 runtime 痕迹必须拒绝或迁移清理。

## Read Next

- 系统总览: [`ARCHITECTURE.md`](ARCHITECTURE.md)
- 文档首页: [`docs/index.md`](docs/index.md)
- 开发宪章: [`.specify/memory/constitution.md`](.specify/memory/constitution.md)
- 运行链路: [`docs/design-docs/runtime-flow.md`](docs/design-docs/runtime-flow.md)
- 模块边界: [`docs/design-docs/module-boundaries.md`](docs/design-docs/module-boundaries.md)
- 迁移收口与 lane-ready 现状: [`docs/design-docs/runtime-kernel-refactor.md`](docs/design-docs/runtime-kernel-refactor.md)
- Prompt / Workspace 上下文: [`docs/design-docs/prompt-and-workspace-context.md`](docs/design-docs/prompt-and-workspace-context.md)
- 配置与环境变量: [`docs/references/configuration.md`](docs/references/configuration.md)
- 测试矩阵: [`docs/references/testing.md`](docs/references/testing.md)
- 工作区布局: [`docs/references/workspace-layout.md`](docs/references/workspace-layout.md)

## Active Technologies

- Go 1.25 + TypeScript/React (existing `web/` client, contract consumer only) + Cobra CLI, `modernc.org/sqlite`, `github.com/openai/openai-go/v3`, `gopkg.in/telebot.v4`, React + Vite (003-four-plane-architecture-refactor)
- SQLite (`workspace/runtime/app.db`) + workspace `memory/` + prompt/workspace/context files under existing safety boundaries (003-four-plane-architecture-refactor)

## Recent Changes

- 003-four-plane-architecture-refactor: Added Go 1.25 + TypeScript/React (existing `web/` client, contract consumer only) + Cobra CLI, `modernc.org/sqlite`, `github.com/openai/openai-go/v3`, `gopkg.in/telebot.v4`, React + Vite
