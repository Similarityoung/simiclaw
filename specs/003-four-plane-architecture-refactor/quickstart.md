# Quickstart: 按四块统一骨架重建 SimiClaw

## Goal

在不改变 SQLite-first、两阶段执行、durable delivery 和 workspace 安全边界的前提下，把后端重构为稳定的四块 owner 骨架，并消除当前混合中心对象。

## Read This First

1. [spec.md](./spec.md)
2. [plan.md](./plan.md)
3. [research.md](./research.md)
4. [data-model.md](./data-model.md)
5. [four-plane-boundaries.md](./contracts/four-plane-boundaries.md)
6. [constitution.md](/Users/similarityyoung/Documents/SimiClaw/.specify/memory/constitution.md)
7. [ARCHITECTURE.md](/Users/similarityyoung/Documents/SimiClaw/ARCHITECTURE.md)
8. [module-boundaries.md](/Users/similarityyoung/Documents/SimiClaw/docs/design-docs/module-boundaries.md)
9. [runtime-flow.md](/Users/similarityyoung/Documents/SimiClaw/docs/design-docs/runtime-flow.md)

## Working Rules

- 四块是 owner map，不是必须同时发生的大搬家。
- 先补 guardrails 和 boundary contracts，再动大块实现。
- 未通过 Phase 2 基线 gate（architecture、docs-style、契约敏感 integration/client/web 验证）前，不进入任何用户故事实现。
- 默认冻结外部可见契约；不在这次架构重构里顺手改 HTTP/SSE/CLI/Web behavior。
- `chat:stream` 这类组合入口必须先拆清 owner：command ingest、runtime observe/replay、SSE framing、client fallback 分别归属不同边界。
- 不引入兼容层、双路径、双写或临时桥接逻辑。
- 每个 slice 都必须做到“新 owner 接管后，旧路径当场清掉或退化为测试辅助”。

## Recommended Slice Order

1. Plane map、contract freeze、architecture tests。
2. Runtime / Capability core decomposition。
3. Context / State separation。
4. Surface adapter convergence。
5. Bootstrap、docs、cleanup closure。

## Where To Start In Code

```text
internal/runtime/
internal/gateway/
internal/runner/
internal/outbound/
internal/store/
internal/query/
internal/prompt/
internal/workspacefile/
internal/http/
cmd/simiclaw/internal/
tests/architecture/
```

第一步不是恢复所有入口，而是先把以下问题钉死：

- 哪些模块属于四块中的哪一块
- Surface 调用哪些显式 command/query/observe seams
- 当前 `chat:stream` 这类组合行为拆完后，哪一段归 command boundary、哪一段归 Runtime、哪一段归 Surface stream adapter、哪一段归 client consumer
- Runtime 如何分别拿 facts、projections、context、capabilities
- 哪些胖对象必须被拆掉，且拆完后由谁接管

## Minimum Validation Commands

文档与边界变化：

```bash
go test ./tests/architecture/... -v
make docs-style
```

常规后端行为变化：

```bash
make test-unit
```

并发、worker、lane、host lifecycle 变化：

```bash
make test-unit-race-core
```

运行语义变化：

```bash
make accept-current
```

Web 可观察契约或客户端 fixture 变化：

```bash
make web-ci
```

契约敏感路径最小验证：

```bash
go test ./tests/integration/... -tags=integration -run 'TestIngestToProcessedAndQuerySQLite|TestChatStreamAcceptedToDone|TestRuntimeTracePathExposesClaimExecuteFinalizeAndDelivery' -v
go test ./cmd/simiclaw/internal/client/... -v
make web-ci
```

## Done Criteria For One Slice

- owner、输入输出契约和失败语义已经写清楚。
- 对应旧路径已经删除或降级为纯测试辅助，不存在长期双路径。
- 至少通过该 slice 所需的最小验证命令。
- 没有引入新的旁路写库、旁路发送或 ad hoc filesystem access。
- 可以在不依赖“下一个 slice 一定完成”的前提下独立回滚。

## Validation Snapshot

### US1 Checkpoint (2026-03-25)

- `go test ./tests/architecture/... -v`: PASS
- `make docs-style`: PASS
- 已固定内容: 四块 owner map、当前可执行依赖方向 guardrails、长期文档、包注释、composition root wiring 说明

### US2 Checkpoint (2026-03-25)

- `go test ./internal/http/... ./cmd/simiclaw/internal/... ./internal/channels/telegram/... -v`: PASS
- `go test ./cmd/simiclaw/internal/client/... -v`: PASS
- `go test ./tests/integration/... -tags=integration -run 'TestIngestToProcessedAndQuerySQLite|TestChatStreamAcceptedToDone|TestRuntimeTracePathExposesClaimExecuteFinalizeAndDelivery|TestHTTPChatStreamTerminalRecordMatchesQueryProjection' -v`: PASS
- `make web-ci`: PASS
- 已固定内容: HTTP `chat:stream` 只消费 command/observe seam，Telegram adapter 只依赖 surface ingress seam，CLI/Web fallback 保持在 consumer 侧

### US3 Checkpoint (2026-03-25)

- `go test ./internal/runtime/... ./internal/runner/... ./internal/outbound/... -v`: PASS
- `go test ./tests/integration/... -tags=integration -run 'TestRuntimeKernelLifecycleStreamAndPersistence|TestRuntimeTracePathExposesClaimExecuteFinalizeAndDelivery' -v`: PASS
- `make test-unit-race-core`: PASS
- `make accept-current`: PASS
- 已固定内容: `ProviderRunner` 收紧为 payload dispatch + memory executor + agent executor；`runnerExecutor` 通过显式 translator / delivery resolver 对接 kernel；`runtimeEventStreamSink` 已拆分 translator + publisher；`events.Hub` 的 publish path 已拆分为 metadata populate + policy + dispatch；`Supervisor` 已收口为 `HostControl` + `ReadinessProbe`，host control 不再与 readiness aggregation 共享同一个公开 owner

### US4 Checkpoint (2026-03-25)

- `go test ./internal/store/... ./internal/query/... ./internal/prompt/... ./internal/workspacefile/... ./internal/tools/... ./internal/memory/... -v`: PASS
- `make test-unit`: PASS
- `make accept-current`: PASS
- 已固定内容: `store/tx` 不再依赖 `store/queries` 获取 event/session 读取面；query service 按 events/runs/sessions 显式拆分 consumer-owned contract；prompt 静态上下文改为只读 bundle 组装；workspace/context 与 curated memory 的整文件读取收敛到 `internal/workspacefile` 与 `internal/memory` 边界

### US5 Checkpoint (2026-03-25)

- `go test ./internal/provider/... ./internal/tools/... ./internal/runner/... -v`: PASS
- `make test-unit`: PASS
- 已固定内容: provider/tool invocation contract 已统一收敛到 capability seam；timeout、typed error 与 trace/log 语义停留在 Runtime <-> Capability 边界；capability source 不再直接推进 durable state

### Phase 8 Final Closure (2026-03-25)

- `go test ./tests/architecture/... -v`: PASS
- `make docs-style`: PASS
- `make test-unit`: PASS
- `make test-unit-race-core`: PASS
- `make accept-current`: PASS
- `make web-ci`: PASS
- 已固定内容: `bootstrap.NewApp` 改为按 plane seam 装配进程；`internal/http/server.go` 只保留 health/command/query/stream route registration；runtime boundary adapters 只负责 runner invocation translation 与 runtime event publish；导航文档、测试矩阵和 quickstart 已与最终代码形状对齐
