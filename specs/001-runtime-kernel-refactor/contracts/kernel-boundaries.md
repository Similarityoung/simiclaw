# Kernel Boundaries Contract

## Purpose

定义 runtime kernel 重构后最先要稳定下来的内部边界。这里不是数据库 schema，也不是外部 HTTP API，而是内核与外部边界/事实层之间的开发契约。

## Boundary 1: Runtime Execution

### Kernel Responsibilities

- 接收 runnable work
- 协调 claim、execute、finalize
- 决定 worker owner 与 lane ownership
- 发布 runtime events / delivery intent

### Required Contract Shape

- `WorkItem`
- `ClaimContext`
- `ExecutionResult`
- `FinalizeCommand`

内核必须依赖 consumer-owned interfaces，而不是直接依赖 store 行级结构。

## Boundary 2: Fact Store Boundary

### Responsibilities

- 提供 claim / finalize / recover / list runnable / job claim / outbox claim 等事实操作
- 将持久化事实映射到 kernel contract types
- 保持 SQLite-first 和单 writer 约束

### Rules

- 事实层实现可以依赖 `internal/store`
- kernel、gateway、http、channels、runner 不得直接依赖 `internal/store`
- 事实层实现不得改变事实表语义

## Boundary 3: Payload Handling

### Responsibilities

- 根据 payload type 选择 handler
- 统一表达 normal / no-reply / background payload 行为
- 让未来新 payload type 通过注册接入

### Rules

- payload dispatch 必须是显式扩展点
- `memory_flush`、`compaction`、`cron_fire` 仍保持 `RunModeNoReply`

## Boundary 4: External Ingress / Egress

### Ingress Contract

- HTTP 和 channels 必须在各自边界把外部消息标准化为统一 ingest request
- HTTP 和 channels 不直接持久化 event
- HTTP 和 channels 不负责 session routing 事实逻辑

### Egress Contract

- HTTP stream 或 channel sender 只消费 durable delivery envelope 或 runtime events
- 外部边界不得绕过 outbox 直接发送业务消息
- channel-specific auth / retry / formatting 归属外部边界实现或 delivery policy

## Boundary 5: Background Workers

### Responsibilities

- heartbeat
- processing recovery
- delivery polling
- scheduled jobs
- future lane coordination

### Rules

- 每个 worker 必须是具名 role
- 每个 worker 必须有 poll cadence、heartbeat name、stop path、failure strategy
- worker 之间共享 kernel contracts，而不是互相直接调用具体实现

## Boundary 6: Concurrency Lanes

### Purpose

为未来的 session serialization 和 named queue model 提供基础抽象。

### Rules

- lane 是 ownership model，不是新的事实源
- lane 设计不得破坏单 writer 和两阶段执行
- 同一 lane 的串行化策略必须显式，而不是隐含在 goroutine 时序中

## Boundary 7: Supporting Package Ownership

### Purpose

防止“主链路已重构，但 supporting code 仍漂在根级 `internal/`”的未收口状态长期存在。

### Rules

- runtime event publication / replay / subscription 归属 `internal/runtime` owner；`internal/http/stream` 只消费 runtime events，不拥有事件总线本体。
- static system prompt assets 归属 `internal/prompt` owner，不单独形成根级 `internal/systemprompt`。
- workspace context read boundary 与 workspace file write boundary 归属同一 `internal/workspacefile` owner，不单独形成根级 `internal/contextfile`。
- CLI-only user-visible message catalogs 归属 `cmd/simiclaw/internal` owner，不单独形成仓库级 `internal/ui`。
- 若某 supporting package 只服务一个明确子系统，则必须并回该 owner；不得以长期根级灰色包形式保留。
