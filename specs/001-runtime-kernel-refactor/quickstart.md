# Quickstart: Runtime Kernel Refactor

## Goal

在不改变 SQLite 事实模型和核心运行不变量的前提下，把当前后端重构为“内核 + 扩展点 + 可替换适配层”的结构。

## Read This First

1. [spec.md](./spec.md)
2. [plan.md](./plan.md)
3. [analysis.md](./analysis.md)
4. [research.md](./research.md)
5. [data-model.md](./data-model.md)
6. [kernel-boundaries.md](./contracts/kernel-boundaries.md)
7. [constitution.md](/Users/similarityyoung/Documents/SimiClaw/.specify/memory/constitution.md)
8. [ARCHITECTURE.md](/Users/similarityyoung/Documents/SimiClaw/ARCHITECTURE.md)
9. [runtime-flow.md](/Users/similarityyoung/Documents/SimiClaw/docs/design-docs/runtime-flow.md)

## Working Rules

- 只强保 SQLite facts 和核心运行不变量；旧 HTTP/CLI/Telegram 实现不是第一阶段兼容边界。
- 先定义 contracts，再改实现。
- 先做 runtime kernel，再做 gateway/routing，再做 http/channels/delivery，最后做 concurrency lanes。
- 不允许为了“先跑起来”而绕过 ingest、claim、finalize、outbox-after-commit 这些硬约束。

## Phase 1 Starting Point

优先从这些文件与目录开始：

```text
internal/runtime/
internal/gateway/
internal/http/
internal/channels/
internal/outbound/
internal/store/
```

第一阶段重点不是“恢复所有现有入口”，而是：
- 定义 kernel contracts
- 拆清 owner
- 建立 migration slice
- 让 store 成为清晰的事实层，而不是中心杂糅点

## Validation Commands

文档与边界变化：

```bash
go test ./tests/architecture/... -v
```

常规后端行为：

```bash
make test-unit
```

并发/worker/lifecycle 变化：

```bash
make test-unit-race-core
```

运行语义变化：

```bash
make accept-current
```

## Done Criteria for a Migration Slice

一个 slice 只有在满足以下条件后才算完成：

- 对应 contracts 已经成文
- owner 和边界清楚
- 至少通过该 slice 需要的最小验证
- 没有引入新的旁路写库或旁路发送
- 可以在不依赖“下一个 slice 一定完成”的情况下回滚
