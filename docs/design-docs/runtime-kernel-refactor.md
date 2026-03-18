# Runtime Kernel Refactor

## Status

- State: active
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

当前已落地第一批基础骨架：

- `internal/runtime/{kernel,payload,workers,lanes}`
- `internal/gateway/{bindings,model,routing}`
- `internal/http/{ingest,query,stream,middleware}`
- `internal/store/{tx,projections,queries}`
- `internal/runner/context`

后续将按 `tasks.md` 的阶段顺序推进具体迁移。
