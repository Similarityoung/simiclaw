# Runtime Kernel Refactor

## Status

- State: completed
- Spec: [`specs/001-runtime-kernel-refactor/spec.md`](/Users/similarityyoung/Documents/SimiClaw/specs/001-runtime-kernel-refactor/spec.md)
- Plan: [`specs/001-runtime-kernel-refactor/plan.md`](/Users/similarityyoung/Documents/SimiClaw/specs/001-runtime-kernel-refactor/plan.md)
- Tasks: [`specs/001-runtime-kernel-refactor/tasks.md`](/Users/similarityyoung/Documents/SimiClaw/specs/001-runtime-kernel-refactor/tasks.md)

## Goal

在不改变 SQLite 事实模型与核心运行不变量的前提下，把现有后端重构为更清晰的：

- `gateway`
- `runtime`
- `http`
- `channels`
- `outbound`
- `store`

目录与职责结构。

## Target Owners

- `internal/gateway/`: 统一 ingress 之后的绑定与路由
- `internal/runtime/`: `claim -> execute -> finalize` 内核与 worker owner
- `internal/http/`: HTTP 写入口、读接口、stream 与 middleware
- `internal/channels/`: Telegram/CLI 等外部通道边界
- `internal/outbound/`: durable delivery、sender、retry
- `internal/store/`: SQLite 事实层、投影与查询

## Migration Principles

1. 不破坏 SQLite-first 与两阶段执行不变量
2. 先定义 contracts，再迁移实现
3. 先让新目录成为 owner，再删除旧路径
4. 每个 slice 都必须能独立验证和回滚

## Current Phase

当前已完成 US4 的迁移收口与 U5 文档/验证收尾，现状如下：

- `internal/runtime/{kernel,payload,workers,lanes}`
- `internal/gateway/{bindings,model,routing}`
- `internal/http/{ingest,query,stream,middleware}`
- `internal/store/{tx,projections,queries}`
- `internal/runner/context`
- `internal/readmodel/` 已删除，read/query DTO 统一收敛到 `internal/query/model`
- `internal/bootstrap/app.go` 只装配 `storetx.RuntimeRepository` 与 `storequeries.Repository`

## US4 Ownership Result

- 写事务 owner: `internal/store/tx`
- session derived projection owner: `internal/store/projections`
- 读查询 owner: `internal/store/queries`
- runner 上下文组装 owner: `internal/runner/context`
- lane-ready hooks owner: `internal/runtime/lanes`

这意味着后续新增 concurrency、delivery、channel 变更时，不需要再回到旧 `internal/store/*.go` 扁平入口做顶层重写。

## US4 Rollback / Validation Checkpoints

- Checkpoint A: 新 repo 形状先接管 bootstrap、query service、runner context。
- Checkpoint B: 删除旧 `internal/store` 扁平实现与 `internal/readmodel/`，生产代码不再保留双路径。
- Checkpoint C: 跑完 `architecture -> unit -> race-core -> targeted integration -> accept-current`，确认 lane hooks 没有破坏两阶段 runtime 生命周期。

推荐验证命令：

```bash
go test ./tests/architecture/... -v
make test-unit
make test-unit-race-core
go test ./tests/integration/... -tags=integration -run 'TestRuntimeTracePathExposesClaimExecuteFinalizeAndDelivery|TestRuntimeLaneHooksPreserveLifecycleAndExposeSessionLane' -v
make accept-current
```
