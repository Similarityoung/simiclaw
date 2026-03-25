# Four-Plane Boundaries Contract

## Purpose

定义四块统一骨架下最先要稳定的边界契约。这里描述的是 owner 和调用规则，不是数据库 schema，也不是对外 wire payload 细节。

## Contract 1: Surface -> Command Boundary

### Responsibilities

- Surface 完成 auth、request decode、normalization、serialization 和 transport-level streaming。
- Surface 通过显式 command boundary 提交 durable command，不直接驱动 Runtime 内核事务。

### Rules

- `internal/http/`、`cmd/simiclaw/`、`internal/channels/` 不得直接 import facts store。
- Surface 只构造 normalized command request，不直接控制 claim/finalize/delivery。
- `idempotency_key`、session hints、caller scope 仍在 command ingress boundary 中被统一解释。

## Contract 2: Surface -> Query Boundary

### Responsibilities

- Surface 的只读入口只通过 query boundary 读取 projections/read models。
- inspect、health、sessions、events、runs 等查询行为不直接走 Runtime state transition。

### Rules

- Query boundary 只读，不推进 durable state。
- 不允许把 query fallback 再塞回 command/stream 入口，形成新的混合中心对象。

## Contract 3: Surface -> Observe Boundary

### Responsibilities

- streaming/SSE/long-poll 等 observe adapter 只负责 transport output。
- Runtime event publication、replay、terminal retention 由 Runtime owner 提供。
- 对 `POST /v1/chat:stream` 这类组合入口，Surface stream adapter 只负责“提交 command -> 订阅 observe -> 输出 SSE frame”的表层组合。

### Rules

- Surface 不拥有 observe event hub。
- Observe 订阅只能消费 runtime events/replay contract，不能反向控制 execution lifecycle。
- command ingest 属于 command boundary，runtime event publication/replay 属于 Runtime，SSE framing 属于 Surface stream adapter。
- CLI / Web / shared client 的 retry、polling fallback、stream unsupported recovery 属于客户端 consumer，不回流到后端 stream handler。

## Contract 4: Runtime -> Context/State Boundary

### Responsibilities

- Runtime 通过显式写事实 contract 推进 durable state。
- Runtime 通过独立的 projection/context contract 读取只读数据和上下文资源。

### Rules

- 禁止混合 write facts、read projections、workspace access 的 god repository。
- workspace safety 仍由 `internal/workspacefile` 等边界 owner 控制，Runtime 不得直接绕过。
- `sessions` 仍只作为 derived cache，而不是主事实源。

## Contract 5: Runtime -> Capability Boundary

### Responsibilities

- Runtime 通过清晰接口调用 provider、tool、skill、MCP、router。
- timeout、retry、error typing 和 trace/log 语义在边界上显式定义。

### Rules

- Capability Plane 不直接写 durable runtime state。
- Runtime 不把 transport-specific behavior 或 workspace filesystem access 塞进 capability implementation。

## Contract 6: Admin Mutation vs Host Control

### Responsibilities

- 持久化状态变化类 admin 动作走 durable command path。
- 进程内 supervision、worker stop/start、readiness aggregation 归属 host control。

### Rules

- 不允许直接拍 `EventLoop` 或内部 worker 来偷偷推进 durable state。
- 只有 process-local host control 可以绕开 durable command ingestion。

## Contract 7: External Contract Freeze

### Rules

- 本次架构重构默认不改变现有 HTTP、SSE、CLI 与 Web 可观察契约。
- 若某个实现变更要求同时改 wire contract，必须新开 spec，而不是在当前 feature 内隐式扩 scope。
- `pkg/api` 继续是对外稳定契约；内部 DTO 只能留在各自 `internal/<subsystem>/model`。
- 代表性现有契约测试必须被固定为 mandatory baseline，至少覆盖 `/v1/events:ingest`、`/v1/chat:stream`、CLI stream fallback 和 web stream consumption。

## Contract 8: `web/` Role

### Rules

- `web/` 是 Surface contract consumer，不是后端 Surface adapter owner。
- `web/` 仅在客户端 contract fixture、API usage 或流式交互消费方式受影响时需要联动验证。
- 不把前端状态管理问题混入后端四块 owner 设计。
