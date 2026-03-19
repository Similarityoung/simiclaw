# Implementation Plan: Runtime Kernel Refactor

**Branch**: `codex/001-runtime-kernel-refactor` | **Date**: 2026-03-19 | **Spec**: [spec.md](./spec.md)
**Input**: Revised feature specification from `/specs/001-runtime-kernel-refactor/spec.md`

## Summary

原始 US1-US4 已完成 runtime kernel、gateway/http/channels、lane-ready hooks 与 store/query 迁移主线，但目录 owner 收口仍未完成。当前剩余问题被定义为“实现未完全收口”，而不是“文档偏移”：`internal/streaming`、`internal/systemprompt`、`internal/contextfile`、`internal/ui/messages` 这四块仍以根级 supporting packages 的形式存在，没有回收到已经确立的 owner 下面。

本次计划只聚焦剩余 owner closure，不重开已完成的运行语义迁移。目标是把：

- runtime event publication / replay 收回 `internal/runtime`
- static system prompt assets 收回 `internal/prompt`
- workspace context read boundary 收回 `internal/workspacefile`
- CLI-only messages 收回 `cmd/simiclaw/internal`

## Technical Context

**Language/Version**: Go 1.25  
**Primary Dependencies**: Cobra CLI, `modernc.org/sqlite`, `github.com/openai/openai-go/v3`, `gopkg.in/telebot.v4`, React + Vite  
**Storage**: SQLite (`workspace/runtime/app.db`) + workspace memory/context files  
**Testing**: `go test ./tests/architecture/... -v`, `make test-unit`, `make test-unit-race-core`, `make accept-current`, targeted package tests  
**Target Platform**: 单机单进程单二进制 backend runtime，带 CLI、HTTP、可选 channel 与 web frontend  
**Project Type**: 单仓库 Go runtime service + CLI + optional web frontend  
**Constraints**: 不改变 SQLite-first、单 writer、两阶段执行、outbox-after-commit、`RunModeNoReply` 特例、现有 HTTP/SSE/prompt 外部行为；不引入兼容层或双路径  
**Scale/Scope**: US5 owner-closure follow-up；仅收口剩余目录 owner，不重新设计已完成的 kernel/gateway/store/query 主链路

## Constitution Check

- `PASS`: 不引入新的 runtime 事实源，也不改变现有事实表语义。
- `PASS`: runtime events 仍由 runtime 产生并供 HTTP stream 消费，不让 HTTP 反向拥有执行语义。
- `PASS`: prompt、workspacefile、cmd 各自只收回本来就单 owner 的 supporting code，不扩大功能范围。
- `PASS`: 不增加兼容层、shim、双写或临时桥接；迁移完成后删除旧包。
- `PASS`: 计划仍按可独立验证的 slice 推进，避免“必须全部做完才可用”。

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

### Target Source Code Shape

```text
cmd/
└── simiclaw/
    └── internal/
        ├── chat/
        ├── gateway/
        ├── initcmd/
        ├── inspect/
        ├── messages/
        ├── root/
        └── version/

internal/
├── bootstrap/
├── gateway/
│   ├── bindings/
│   ├── model/
│   └── routing/
├── runtime/
│   ├── events/
│   ├── kernel/
│   ├── lanes/
│   ├── model/
│   ├── payload/
│   └── workers/
├── runner/
│   ├── context/
│   ├── model/
│   └── tools/
├── http/
│   ├── ingest/
│   ├── middleware/
│   ├── query/
│   └── stream/
├── channels/
│   ├── cli/
│   ├── common/
│   └── telegram/
├── outbound/
│   ├── delivery/
│   ├── retry/
│   └── sender/
├── store/
│   ├── projections/
│   ├── queries/
│   └── tx/
├── query/
│   └── model/
├── prompt/
├── memory/
├── provider/
├── tools/
├── workspace/
├── workspacefile/
└── config/
```

**Packages To Eliminate**:

- `internal/streaming`
- `internal/systemprompt`
- `internal/contextfile`
- `internal/ui/messages`

**Structure Decision**: 根级 `internal/` 只保留长期子系统 owner。只服务单一 owner 的 supporting code 必须并回该 owner，而不是继续作为漂浮的根级包存在。

## Follow-up Slices

### Slice 1 - Runtime Event Ownership Closure

目标：把 runtime event hub 从 `internal/streaming` 收回 `internal/runtime/events`，明确 runtime 才是 event publication / replay 的 owner，`internal/http/stream` 只作为 SSE 适配层存在。

范围：

- 迁移 hub、subscription、replay、terminal retention 相关实现与测试
- 更新 bootstrap、HTTP stream handler 和相关测试引用
- 保持 `RuntimeEvent` 类型与 HTTP stream 消费语义不变

验证：

- `go test ./tests/architecture/... -v`
- `go test ./internal/runtime/... ./internal/http/...`
- 必要时运行受影响 integration tests

### Slice 2 - Prompt And Workspace Boundary Closure

目标：把 static system prompt assets 并入 `internal/prompt`，把 workspace context read boundary 并入 `internal/workspacefile`，让 prompt builder、prompt fingerprint 和 `context_get` 共享同一 owner 边界。

范围：

- 迁移 system prompt 的 embed 与模板文件加载逻辑
- 迁移 context file whitelist、path resolution、range read 逻辑
- 保持 prompt section 顺序、context 白名单与 fingerprint 语义不变

验证：

- `go test ./internal/prompt/... ./internal/tools/... ./internal/workspacefile/...`
- `go test ./tests/architecture/... -v`
- 必要时运行 prompt / workspace 相关 targeted tests

### Slice 3 - Command Message Ownership Closure

目标：把 CLI/chat/inspect 的用户可见文案从 `internal/ui/messages` 下沉到 `cmd/simiclaw/internal/messages`，明确它属于命令层展示资源，而不是仓库级 UI 子系统。

范围：

- 迁移 chat/root/init/inspect/version/gateway 命令使用的 messages 包
- 保持 Cobra help、chat TUI/REPL、inspect 输出行为不变
- 不改 `web/`，也不把 web/telegram 文案强行汇总到同一包

验证：

- `go test ./cmd/simiclaw/internal/...`
- `go test ./tests/architecture/... -v`

### Slice 4 - Guardrails, Deletion, And Final Closure

目标：补上架构守护与文档，删除旧包，确保 owner closure 可持续。

范围：

- 更新 architecture tests，明确新的 owner 边界
- 更新设计文档和导航，反映最终目录 owner 形状
- 删除旧包与旧 import，确保无双路径残留

验证：

- `go test ./tests/architecture/... -v`
- `make test-unit`
- `make test-unit-race-core`
- 如 runtime / SSE 行为受影响，运行 `make accept-current`

## Validation Strategy

- 目录 owner 与 import 方向变化：至少运行 `go test ./tests/architecture/... -v`
- runtime event hub 迁移：至少运行 `go test ./internal/runtime/... ./internal/http/...`
- prompt / context 边界迁移：至少运行 `go test ./internal/prompt/... ./internal/tools/... ./internal/workspacefile/...`
- 命令层文案迁移：至少运行 `go test ./cmd/simiclaw/internal/...`
- Slice 全部完成后：运行 `make test-unit`；如流式/运行链路受影响，再运行 `make accept-current`

每个 follow-up slice 都必须满足：

- 不改变外部行为
- 没有留下旧新双路径
- owner 归属比迁移前更明确
- 能独立验证和回滚

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| runtime event hub 迁移后 HTTP stream 语义退化 | 高 | 只改 owner，不改事件模型；保留 replay / terminal 语义并补 targeted tests |
| `contextfile` 并入 `workspacefile` 时放宽白名单或越界校验 | 高 | 先迁测试，再迁实现；白名单、symlink 校验、range read 保持逐项等价 |
| `systemprompt` 并入 `prompt` 后 prompt cache / fingerprint 意外变化 | 中 | 保持模板内容、加载顺序和 fingerprint 输入不变，补 builder/fingerprint tests |
| CLI messages 下沉导致命令层 import 面变大 | 中 | 只在 `cmd/simiclaw/internal` 内部消费新包，避免反向给 runtime/http 使用 |
| 只做包移动，不补守护，之后再次漂移 | 高 | 在 architecture tests 与 design docs 中把 owner closure 明确固化 |

## Complexity Tracking

本次修订不重开原始 runtime kernel 重构，只处理未收口的 supporting package owner。复杂度来自 package move 与 guardrail 补齐，而不是运行语义变化。
