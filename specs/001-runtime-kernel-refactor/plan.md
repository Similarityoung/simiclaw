# Implementation Plan: Runtime Kernel Refactor

**Branch**: `codex/001-runtime-kernel-refactor` | **Date**: 2026-03-18 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-runtime-kernel-refactor/spec.md`

## Summary

本次重构以“保留 SQLite 事实模型和核心运行不变量，重建可扩展 Agent Runtime 内核”为目标。实施方式采用分阶段迁移而不是全量推倒：先建立新的 runtime kernel 契约和扩展接口，再重建 gateway/routing、http/channels/delivery，最后引入 lane-ready 的并发内核。

本计划的核心策略来自 research 结论：
- 不改变 `events` / `runs` / `messages` / `outbox` / `scheduled_jobs` / `heartbeats` 的事实语义。
- 不改变 claim -> 事务外执行 -> finalize 的两阶段运行模型。
- 当前 HTTP、CLI、Telegram 外部接入实现视为可替换层，不作为第一阶段保留目标。
- 所有新增能力都先进入显式扩展点，而不是继续堆进中心文件。

## Technical Context

**Language/Version**: Go 1.25  
**Primary Dependencies**: Cobra CLI, `modernc.org/sqlite`, `github.com/openai/openai-go/v3`, `gopkg.in/telebot.v4`, React + Vite (现有前端，非第一阶段重点)  
**Storage**: SQLite (`workspace/runtime/app.db`) + workspace memory/context files  
**Testing**: `go test ./tests/architecture/... -v`, `make test-unit`, `make test-unit-race-core`, `make accept-current`, targeted integration tests  
**Target Platform**: 单机单进程单二进制 backend runtime，运行于本地 workspace，带可选 HTTP 与 channel 接入  
**Project Type**: 单仓库 Go runtime service + CLI + optional web frontend  
**Performance Goals**: 保持当前 V1 正确性与持久化语义；为后续 bounded worker、lane serialization 和可扩展 channel/delivery 奠定内核，不引入新的无界 goroutine fan-out  
**Constraints**: 保留 SQLite-first、单 writer、两阶段执行、outbox-after-commit、`RunModeNoReply` 特例、`internal/store` 边界约束；迁移必须分 slice 验证与回滚  
**Scale/Scope**: V1 后端内核重构，覆盖 runtime/gateway/channels/outbound/concurrency foundations；前端重做不在本计划范围内

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- `PASS`: 继续以 SQLite 作为唯一事实源，不引入新的 runtime 事实存储。
- `PASS`: 保留两阶段执行和 durable delivery 语义，所有真实发送仍晚于 outbox 持久化。
- `PASS`: 重构目标明确要求“先定义扩展点再重挂外部边界实现”，符合 explicit extension points 原则。
- `PASS`: 保持 `pkg/api`、`pkg/model`、`internal/<subsystem>/model` 的边界分层，不让 `store` 类型穿透上层。
- `PASS`: 计划按迁移 slice 推进，每一阶段都带验证门槛，符合 reversible change 与 test-gated change 要求。
- `PASS`: 并发设计以后续 lane / owner / bounded worker 为中心，不允许无界 goroutine 扩散。

## Project Structure

### Documentation (this feature)

```text
specs/001-runtime-kernel-refactor/
├── analysis.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── kernel-boundaries.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/
└── simiclaw/

internal/
├── bootstrap/
├── gateway/
│   ├── bindings/
│   ├── model/
│   └── routing/
├── runtime/
│   ├── kernel/
│   ├── payload/
│   ├── workers/
│   ├── lanes/
│   └── model/
├── runner/
│   ├── context/
│   ├── tools/
│   └── model/
├── http/
│   ├── ingest/
│   ├── query/
│   ├── stream/
│   └── middleware/
├── channels/
│   ├── cli/
│   ├── telegram/
│   └── common/
├── outbound/
│   ├── delivery/
│   ├── sender/
│   └── retry/
├── store/
│   ├── tx/
│   ├── projections/
│   └── queries/
├── query/
│   └── model/
├── prompt/
├── memory/
├── tools/
├── provider/
├── workspace/
├── workspacefile/
└── config/

pkg/
├── api/
└── model/

tests/
├── architecture/
├── integration/
└── e2e/
```

**Structure Decision**: 保持单仓库、单后端项目结构，不拆成多个独立服务。保留 `store`、`runner`、`query`、`prompt`、`memory` 等稳定子系统，重点把 `runtime`、`gateway`、`http`、`channels`、`outbound` 重组为“内核 + 外部边界 + 事实层”结构。实现时可以先以文件级重组开始，再视复杂度进入子包级重组。

## Phase Plan

### Phase 0 - Research and Contract Freeze

目标：明确重构边界、迁移顺序和内核契约，冻结必须保留的不变量。

产物：
- `research.md`
- `data-model.md`
- `contracts/kernel-boundaries.md`

出口条件：
- 团队对“保留什么、可以重写什么、第一阶段不保什么”没有歧义。
- 新内核的主要接口边界有书面约定。
- 宪章检查通过，且没有未处理的硬性冲突。

### Phase 1 - Runtime Kernel Slice

目标：把当前执行主线重构成显式 kernel 契约与用例中心。

范围：
- 定义新的 kernel contracts：claim source、executor、finalizer、delivery intent、worker lifecycle。
- 将现有 `eventloop`、`workers`、finalize 映射拆成清晰 owner，并把运行事件与 HTTP/SSE 之类的外部传输边界分开。
- 建立第一版 lane-ready 调度点，但暂不引入完整 lanes 并发策略。
- 保持 store 作为事实层，不改事实表语义。

验证：
- `go test ./tests/architecture/... -v`
- `make test-unit`
- `make test-unit-race-core`
- 必要时 `make accept-current`

### Phase 2 - Gateway and Routing Slice

目标：把写入入口和会话绑定规则重建为可扩展 routing pipeline。

范围：
- 定义统一 ingress 之后的 routing/binding 接口；原始输入的 normalize 归属 HTTP/channel 边界，不放进 gateway。
- 从 `gateway` / `ingest` 中收拢 session scope、binding、hint 和 route resolution 语义；现有 `internal/session` 的 key/scope 规则并入 `gateway/bindings`。
- 为未来多级绑定和 session isolation 策略预留显式扩展位。

验证：
- `go test ./tests/architecture/... -v`
- `make test-unit`
- gateway / ingest 相关 targeted tests

### Phase 3 - Channels and Delivery Slice

目标：重建 HTTP/channel 外部边界与 durable delivery 层。

范围：
- 把 HTTP、CLI、Telegram 等外部边界视为基于 contracts 的 replaceable layers。
- 重建 sender/router/retry 结构，使 delivery policy 可插拔。
- 重新接回当前需要保留的 HTTP、CLI、Telegram 能力，不为暂时不用的 channel 预埋空目录。

验证：
- `make test-unit`
- `make test-integration`
- 与 delivery / channel 相关的 acceptance scope

### Phase 4 - Concurrency Lanes Slice

目标：在不破坏单 writer 和两阶段语义的前提下引入命名车道和 session serialization。

范围：
- 设计 lane key、owner 和 bounded worker coordination。
- 让 runtime kernel 能表达“不同 lane 并发、同一 session 或同一 lane 串行”。
- 对 recovery、cron、retry、outbox worker 的 owner 模型统一化。

验证：
- `make test-unit`
- `make test-unit-race-core`
- 必要时 `make accept-current`

## Validation Strategy by Slice

- 文档与结构边界变化：至少运行 `go test ./tests/architecture/... -v`
- runtime/store/ingest/outbound 语义变化：至少运行 `make accept-current`
- 并发、worker、goroutine 生命周期变化：至少运行 `make test-unit-race-core`
- HTTP/channel/delivery 重挂：至少运行 `make test-integration`

每个 migration slice 都必须满足：
- 事实表语义未漂移
- 运行不变量未破坏
- 文档和代码入口同步更新
- 能独立回滚，不依赖“下一阶段一定完成”才能恢复可用性

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| 只改结构不改 owner，最终仍退化为中心文件 | 高 | 每个 slice 必须明确 owner、契约和接入点，并在 tasks 中按 owner 拆分 |
| 迁移期同时保留新旧路径导致双写或重复发送 | 高 | 所有切换点围绕 durable intent 和 single writer 设计，禁止双路径同时提交副作用 |
| 过早重挂 HTTP 或 channel 边界导致内核契约反复变化 | 中 | 先完成 kernel contracts，再按 phase 逐层重挂 gateway/http/channels |
| lanes 设计过早落地引入新的并发错误 | 中 | 在 kernel slice 只预留 lane-ready hooks，不提前引入完整并发策略 |
| 文档与 tasks 脱节 | 中 | `research.md`、`data-model.md`、`contracts/` 和 `tasks.md` 同步维护，以 spec 为单一事实入口 |

## Complexity Tracking

本计划当前无宪章违规项；复杂度来源是迁移编排，而不是架构特权。任何后续需要突破宪章的方案，必须先在本节补充理由、被拒绝的简单替代方案和新增验证要求。
