# Runtime Flow

## Summary

SimiClaw 的主链路是一个典型的“先持久化、后执行、再结果提交”的两阶段 runtime：event 先进入 SQLite，再由 EventLoop 调度 work、按 lane 串行化，最终交给 kernel 执行 claim -> execute -> finalize。

## Context

这条链路的目标是同时满足三件事：幂等接收、可恢复执行和可查询结果。对应的核心包是 `internal/gateway`、`internal/http`、`internal/runtime`、`internal/runner` 和 `internal/store`。

## Details

| 阶段 | 主要包 | 产物 | 关键约束 |
| --- | --- | --- | --- |
| 入口接收 | `internal/http`, `internal/channels/telegram`, `internal/gateway` | 统一 ingress 请求 | `/v1/events:ingest` 必须带 `idempotency_key` |
| 校验与持久化 | `internal/gateway`, `internal/gateway/bindings`, `internal/store/tx`, `internal/store/projections` | `events`, `idempotency_keys`, session scope 信息 | 先持久化 event，再尝试 enqueue |
| claim 与 started run | `internal/runtime/eventloop.go`, `internal/runtime/kernel`, `internal/runtime/lanes`, `internal/store/tx` | `runs(started)`、event claim、lane lease | event 只有 claim 成功后才能进入 `processing`；同 lane work 必须串行 |
| prompt / provider / tools | `internal/runner`, `internal/prompt`, `internal/provider`, `internal/tools` | LLM 输出、tool 执行记录、runtime events、trace | 执行发生在写事务外；最多走 `maxToolRounds` 轮 |
| 结果提交 | `internal/runtime/kernel`, `internal/store/tx`, `internal/store/projections` | `messages`, `runs`, `sessions`, `events`, `outbox`, `jobs` | 所有结果在一个 finalize 事务里提交 |
| 提交后发送与流式通知 | `internal/outbound`, `internal/runtime/events`, `internal/http/stream`, `internal/query`, `internal/store/queries` | durable delivery、runtime event stream、SSE、read DTO | 真实出站发送只能晚于 outbox 持久化；HTTP stream 只消费 runtime events |

### 1. Ingest 接收

- `gateway.Service` 会校验 source、conversation、payload、UTC timestamp 和 `idempotency_key`。
- 请求在进入存储前会被计算 canonical payload hash，并通过 `gateway/bindings.ComputeKey` 生成 `session_key`。
- 只有持久化成功后，event 才可能进入队列。

### 2. EventLoop 与 Lane 调度

- `EventLoop` 同时消费内存队列和 SQLite 中的 runnable event。
- `processWork` 会先补齐 `session_key` / `lane_key`，再通过 `lanes.Scheduler` 保证同 lane work 串行执行。
- 真正的 claim -> execute -> finalize 编排由 `kernel.Service` 持有；`EventLoop` 负责 work 调度、lease 生命周期和 repump。
- 若 `runner` panic 或报错，finalize 仍会把 run / event 标记为失败并写入错误块。

### 3. Runner 执行

- 普通 payload 走 `RunModeNormal`。
- `memory_flush`、`compaction`、`cron_fire` 走 `RunModeNoReply`。
- `cron_fire` 会进入受限工具集合，并对最终输出做 suppress；`memory_flush` / `compaction` 还会把内容写回 memory 文件。
- runner 的 text delta、tool start/result 会被转换成 `RuntimeEvent` 发布到 `internal/runtime/events.Hub`；`internal/http/stream` 再把这些 runtime events 映射成 SSE。

### 4. Finalize 与查询

- finalize 结果里会带上 provider / model / token usage / finish reason / tool calls / diagnostics。
- `sessions` 在提交时被更新，但它仍然只是 derived cache。
- 查询侧统一由 `query.Service` 暴露给 `internal/http/query` 和 `inspect`，底层通过 `internal/store/queries` 生成 `internal/query/model` DTO，避免外部直接触达 store 内部结构。

## Verification

- 应用装配: `internal/bootstrap/app.go`
- ingress 校验与持久化: `internal/gateway/service.go`
- EventLoop 调度与 lane hooks: `internal/runtime/eventloop.go`, `internal/runtime/lanes/*.go`
- kernel 两阶段处理: `internal/runtime/kernel/service.go`
- provider / tools / run mode: `internal/runner/runner.go`
- schema 与事实表: `internal/store/schema.sql`, `internal/store/tx/*.go`, `internal/store/projections/*.go`, `internal/store/queries/*.go`

## Related Docs

- 系统总览: [`../../ARCHITECTURE.md`](../../ARCHITECTURE.md)
- 模块边界: [`module-boundaries.md`](module-boundaries.md)
- Prompt / Workspace: [`prompt-and-workspace-context.md`](prompt-and-workspace-context.md)
- 配置参考: [`../references/configuration.md`](../references/configuration.md)
