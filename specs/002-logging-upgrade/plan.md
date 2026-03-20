# Implementation Plan: 日志系统升级与链路观测补齐

**Branch**: `002-logging-upgrade` | **Date**: 2026-03-20 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/002-logging-upgrade/spec.md`

## Summary

本次计划只解决“运行日志可读性差、关键链路日志点不足”这两个问题，不改运行时语义和对外契约。推荐实现路径固定为：

1. 在 `pkg/logging` 内把当前 zap console encoder 输出改成统一的人类可读单行日志格式，保留结构化字段语义。
2. 沿主链路补齐关联日志：`HTTP/channel ingest -> gateway persist/enqueue -> runtime event loop/kernel -> runner/provider/tools -> finalize -> outbound/workers`。
3. 为高风险字段加摘要/脱敏规则，并在同一变更里补测试与文档。

整个改动继续复用现有 `pkg/logging`，不新增 `log_format` 配置，不引入新的日志依赖，不增加任何兼容层或双路径。

## Target Behavior

- 运行日志改为人类可读的单行文本，字段区不再以 JSON-like 片段作为主要展示形式。
- `info` 级别能串起一次 event 的主链路；`debug` 级别补充高频中间状态，但不把空转和 keepalive 刷到 `info`。
- 每条关键日志尽量带上当前层已有的关联字段，如 `event_id`、`run_id`、`session_id`、`channel`、`outbox_id`、`job_id`、`tool_call_id`。
- 错误只在最有上下文的一层记录一次；其余层返回 rich error，不重复刷 `ERROR`。
- prompt、私有 memory、鉴权信息和大型 tool payload 只以摘要或截断形式出现，不直接打印原文。

## Non-Goals

- 不修改 `/v1/**`、`inspect`、SSE、run trace、SQLite 中各类 `*_json` 字段的契约或存储格式。
- 不引入日志文件落盘、远端 collector、metrics/tracing 系统，或新的 observability 子系统。
- 不记录逐 token / 逐 chunk 的 streaming 日志，不把 text delta 当成运行日志主视图。
- 不做仓库结构重组，不新建“日志服务层”“日志上下文中间件”等超出当前问题规模的抽象。

## Technical Context

**Language/Version**: Go 1.25  
**Primary Dependencies**: `go.uber.org/zap`, Cobra, `modernc.org/sqlite`, `github.com/openai/openai-go/v3`, `gopkg.in/telebot.v4`  
**Storage**: stdout/stderr 运行日志；SQLite runtime 事实表保持不变  
**Testing**: `make fmt`, `go test ./pkg/logging/...`, `go test ./tests/architecture/... -v`, `make test-unit`, `make accept-current`  
**Target Platform**: 单机单进程 Go runtime service + CLI + optional Telegram channel  
**Project Type**: 单仓库后端 runtime/CLI 服务  
**Performance Goals**: `info` 级别不出现高频空转刷屏；日志渲染不引入与 payload 大小线性耦合的原文输出  
**Constraints**: 保持 SQLite-first、两阶段执行、outbox-after-commit、现有 `log_level`、现有 API/trace 契约；不新增兼容层、双格式切换或新依赖  
**Scale/Scope**: 以 `pkg/logging` 为中心，联动 `cmd/`, `internal/http`, `internal/gateway`, `internal/runtime`, `internal/runner`, `internal/provider`, `internal/outbound`, `internal/channels/telegram` 若干调用点与测试文档

## Constitution Check

- `PASS`: 不引入新的 runtime 事实源，也不修改 SQLite 表语义。
- `PASS`: 不改变 claim -> execute -> finalize 两阶段语义，也不改变 outbox-after-commit。
- `PASS`: 继续使用统一结构化日志入口 `pkg/logging`，不退回 ad hoc print logging。
- `PASS`: 不新增兼容层、双格式输出或回退路径。
- `PASS`: 敏感信息默认不进入日志正文，符合“可观测但可控”的治理要求。

## Project Structure

### Documentation (this feature)

```text
specs/002-logging-upgrade/
├── spec.md
├── plan.md
├── research.md
├── quickstart.md
└── tasks.md
```

### Target Source Code Shape

```text
pkg/
└── logging/

cmd/
└── simiclaw/
    └── internal/
        └── gateway/

internal/
├── http/
│   ├── ingest/
│   ├── middleware/
│   └── stream/
├── gateway/
├── runtime/
│   ├── kernel/
│   └── workers/
├── runner/
├── provider/
├── outbound/
│   └── delivery/
└── channels/
    └── telegram/

docs/
└── references/
    └── configuration.md
```

**Structure Decision**: 这是一次以 `pkg/logging` 为中心的跨链路补点改动。实现方式保持“统一 logger 核心 + 精准调用点调整”，不新建顶层包，不改变依赖方向，不把日志上下文抽成新的框架层。

## Canonical Log Line Contract

目标日志行统一为“固定前缀 + 人类可读字段区”的单行形式，例如：

```text
2026-03-20T14:15:16.789+0800 INFO internal/gateway/service.go:173 [gateway] ingest accepted event_id=evt_123 session_key=telegram:dm:42 duplicate=false enqueued=true
```

约束：

- 保留时间、级别、caller 和现有 `[module] message` 风格。
- 字段区采用稳定 `key=value` 文本，而不是 JSON-like blob。
- 字段顺序优先保证关联 ID 和结果字段靠前，便于 grep 与人工扫描。
- 值需要可转义、可截断；复杂对象必须转成摘要而不是原样打印。

## Implementation Slices

### Slice 1 - 统一日志渲染与字段约定

**Goal**: 在不改调用方 API 的前提下，把当前 logger 输出改成统一的人类可读格式，并用测试固定渲染契约。

**Likely Files**:

- `pkg/logging/logger.go`
- `pkg/logging/logger_test.go`
- `pkg/logging/logger_internal_test.go`

**Changes**:

- 用最小改动替换当前 console encoder 的字段渲染方式，使字段区输出为稳定的 `key=value` 文本。
- 保留现有 `Init`、`ParseLevel`、`L(module)`、`With`、`Sync` 和 caller 输出约定，避免影响全部调用点。
- 仅在出现重复需求时增加最小 helper；避免为了“优雅”一次性扩充大量字段 helper。
- 为字符串转义、错误字段、布尔/数值字段和空字段输出补足测试，确保不同模块日志样式一致。

**What Stays Fixed**:

- logger 仍输出到 stdout。
- `log_level` 行为不变。
- module 前缀仍然留在 message 中，而不是迁移成单独顶层字段。

**Verification**:

- `go test ./pkg/logging/...`

### Slice 2 - Ingest / Runtime / Outbound 主链路补点

**Goal**: 先把最关键、最常排障的主链路打通，让一条 event 可以从接收一路追到完成或发送。

**Likely Files**:

- `cmd/simiclaw/internal/gateway/command.go`
- `internal/http/ingest/handler.go`
- `internal/http/middleware/api_key.go`
- `internal/http/stream/handler.go`
- `internal/gateway/service.go`
- `internal/runtime/eventloop.go`
- `internal/runtime/kernel/service.go`
- `internal/outbound/delivery/worker.go`

**Changes**:

- 在 `serve` 启动、关键子系统启动失败、关闭路径上补 lifecycle 日志。
- 在 HTTP ingress/gateway 侧明确记录：API key 鉴权拒绝、解码失败、normalize 失败、validate/binding/routing 失败、rate limit、persist 成功、duplicate、enqueue 成功/失败。
- 在 `EventLoop` 和 `kernel` 侧补：enqueue 接收/丢弃、repump 摘要、claim 成功、执行开始、finalize 开始、finalize 成功/失败。
- 在 outbound 侧把已有日志补齐成统一字段集合，并明确区分 send failed、retry scheduled、dead-letter、complete。

**Edge Handling**:

- “已持久化但未成功 enqueue”必须与“执行已开始”分开记录。
- `duplicate`、`rate limited`、`invalid argument` 等拒绝类结果默认记为 `WARN` 或 `INFO`，不统一抬升成 `ERROR`。
- `http/stream` 的 keepalive、空轮询、连接空转不进 `info`；连接 attach/detach 仅在需要时进入 `debug` 或简洁 `info`。

**Verification**:

- `go test ./internal/gateway/... ./internal/http/... ./internal/runtime/... ./internal/outbound/...`
- `go test ./tests/architecture/... -v`

### Slice 3 - Runner / Provider / Tools / Workers 摘要日志与脱敏

**Goal**: 补齐事务外执行链路和后台 worker 的诊断能力，同时把脱敏与摘要规则收口，避免日志一多就泄漏内容。

**Likely Files**:

- `internal/runner/runner.go`
- `internal/runner/tool_executor.go`
- `internal/provider/openai.go`
- `internal/provider/openai_stream.go`
- `internal/runtime/supervisor.go`
- `internal/runtime/workers/processing_recovery.go`
- `internal/runtime/workers/scheduled_jobs.go`
- `internal/channels/telegram/runtime.go`

**Changes**:

- runner 记录 payload plan、run mode、provider/model、provider 调用开始/结束/超时/失败、tool rounds、finish reason、token usage、latency 和 terminal outcome，并作为携带 `event_id/run_id/session_id` 的终态 provider 日志 owner。
- tool 执行记录 start/finish/deny/fail，但参数与结果只输出摘要，如 key 数、截断预览、是否为空、是否被截断。
- provider 侧优先返回 rich error 与 provider request id / model 等可复用信息；只有在不与 runner 终态日志重复、且确实增加底层 transport 诊断价值时，才补充局部 `debug` 级日志，不打印 prompt 正文。
- processing recovery / scheduled jobs / supervisor 补上回收数量、claim 结果、重入队结果、job ingest 结果等日志。
- Telegram 现有日志保持，但统一字段名和级别语义，避免 channel 成为另一套格式。

**Placement Rule**:

- 脱敏或摘要 helper 优先放在实际使用它的子系统内；只有在多个层都需要并且语义稳定时，才提升到 `pkg/logging`。
- 不创建新的全局 observability util 包。

**Verification**:

- `go test ./internal/runner/... ./internal/provider/... ./internal/runtime/... ./internal/channels/telegram/...`
- `make test-unit`

### Slice 4 - 文档、回归验证与噪声收口

**Goal**: 把最终行为固定到文档和验证命令里，同时收掉过噪或重复的日志点，避免“补了很多日志但不可用”。

**Likely Files**:

- `docs/references/configuration.md`
- 受影响包内测试文件

**Changes**:

- 更新配置文档，明确当前默认日志输出为人类可读单行文本，并说明 `log_level` 作用边界。
- 根据实际实现补充 targeted tests，重点固定 logger 输出、脱敏规则和代表性主链路日志点。
- 复查重复错误日志和过噪 `info` 日志，把空转、keepalive、细粒度调度信息压回 `debug`。

**Verification**:

- `make fmt`
- `go test ./tests/architecture/... -v`
- `make test-unit`
- `make accept-current`

## Validation Strategy

- Logger 核心改动先用 `go test ./pkg/logging/...` 固定格式契约，再扩到调用方。
- 跨包调用点改动以包级 `go test` 和 `make test-unit` 为主，避免把日志断言做成脆弱的全链路 golden。
- 架构边界虽然不直接变化，但改动横跨 `cmd/`, `internal/http`, `internal/runtime`, `internal/runner`, `internal/provider`, `internal/outbound`，仍至少跑一次 `go test ./tests/architecture/... -v`。
- 最终合并前跑 `make accept-current`，确认日志改动没有间接破坏 ingest/runtime/outbound 主链路。

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| 自定义人类可读渲染后，字段转义或排序不稳定 | 高 | 先在 `pkg/logging` 用集中测试固定 line shape、错误字段和特殊字符行为 |
| 跨层补点后，同一失败被重复打印多次 | 高 | 每个 slice 都按“哪一层最有上下文就在哪一层记录”复查，低层只返回错误 |
| 日志一多后泄漏 prompt、token、私有 memory 或工具大 payload | 高 | 默认只打摘要，敏感字段显式 redact/truncate，并补针对性测试 |
| worker 空转、SSE keepalive、streaming 中间态把 `info` 刷爆 | 中 | 明确 `info`/`debug` 分层，不记录逐 tick/逐 chunk 明细 |
| 为了补日志引入新的 helper/抽象，反而扩大改动面 | 中 | 只在重复模式真正出现时增加最小 helper；优先复用现有 `logging.L(...).With(...)` |

## Complexity Tracking

本计划不接受额外复杂度豁免。

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| None | N/A | N/A |
