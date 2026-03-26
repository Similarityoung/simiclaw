# Implementation Plan: 按四块统一骨架重建 SimiClaw

**Branch**: `003-four-plane-architecture-refactor` | **Date**: 2026-03-23 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/003-four-plane-architecture-refactor/spec.md`

## Summary

本次计划把 `spec.md` 中的“四块统一骨架”落实为一个可分阶段执行的重构路径：在不改变 SQLite-first、两阶段执行、outbox-after-commit、`RunModeNoReply` 特例和 workspace 安全边界的前提下，重写模块 owner 与调用关系，消除当前混合中心对象。

计划阶段补了两个必须先钉死的解释：

- `web/` 视为消费 Surface 契约的客户端，不是后端 runtime 内的 transport adapter owner；后端的 Surface owner 仍是 `cmd/simiclaw/`、`internal/http/` 和 `internal/channels/`。
- 这次重构默认冻结现有外部可见 HTTP/SSE/CLI/Web 可观察契约；若将来要改 wire contract，需要单独出 spec，而不是在这次架构重构里顺手改变。
- 当前 `chat:stream` 这类组合行为将被拆成 4 个显式 owner：command ingress、runtime observe/replay、服务端 stream frame 组装、客户端 retry/poll fallback；不再允许单一模块同时拥有这 4 类职责。

推荐实施顺序固定为：

1. 先定义四块 owner map、use-case boundary 和架构 guardrails。
2. 再拆 Runtime 与 Capability 的执行中心对象。
3. 再拆 Context/State，把 facts、projections、context 读写边界拉开。
4. 然后让 Surface adapters 全部改走显式 command/query/observe seams。
5. 最后收装配、文档、测试与遗留包删除。

## Target Behavior

- 维护者可以把任一模块明确归到 `Surface`、`Runtime`、`Context/State`、`Capability Plane` 之一，并说明它允许依赖谁、禁止拥有谁的状态。
- HTTP、CLI、Telegram 等 Surface 不再直接定义系统核心行为，只负责 transport、auth、request normalization、serialization 和 streaming protocol output。
- Runtime 只负责 command 执行、worker lifecycle、delivery coordination、observe publication 和 host control，不再混入 transport 协议、workspace 读写或 capability 选择细节。
- Facts、projections、context assets 不再通过一个“什么都能拿”的仓库接口暴露；Runtime 通过显式边界分别消费写事实、读模型和上下文组装能力。
- tools、providers、future skills/MCP/router 统一作为 Capability Plane 被调用，不允许直接推进 durable state。
- 当前 `chat:stream` 的用户可见语义会被拆到明确 owner：command ingress 属于 command boundary，runtime events/replay 属于 Runtime，SSE frame 组装属于 `internal/http/stream`，CLI/Web 的 retry/poll fallback 属于各自 client consumer。

## Non-Goals

- 不修改 SQLite schema，也不改变 `events`、`runs`、`messages`、`outbox`、`scheduled_jobs`、`heartbeats`、`sessions` 的持久化含义。
- 不把这次重构扩展成新的消息总线、分布式调度、远程 queue、双写兼容层或临时桥接层建设。
- 不顺手修改 HTTP 路由、SSE payload、CLI 文案协议或 Web 消费契约，除非某个后续 spec 明确要求。
- 不为了“四块更纯粹”而强行引入新的顶层 package 命名体系；如现有顶层目录已经足够表达 owner，则优先保留认知成本更低的路径。

## Technical Context

**Language/Version**: Go 1.25 + TypeScript/React (existing `web/` client, contract consumer only)  
**Primary Dependencies**: Cobra CLI, `modernc.org/sqlite`, `github.com/openai/openai-go/v3`, `gopkg.in/telebot.v4`, React + Vite  
**Storage**: SQLite (`workspace/runtime/app.db`) + workspace `memory/` + prompt/workspace/context files under existing safety boundaries  
**Testing**: `make fmt`, `go test ./tests/architecture/... -v`, `make test-unit`, `make test-unit-race-core`, `go test ./tests/integration/... -tags=integration -v`, `make accept-current`, `make web-ci` when web-visible contracts are touched  
**Target Platform**: 单机单进程单二进制 Go runtime service + CLI + optional Telegram channel + separate web client  
**Project Type**: 单仓库 Go backend/runtime + CLI + web client  
**Performance Goals**: 不破坏当前 event ingest -> claim -> execute -> finalize -> delivery 吞吐与恢复语义；不增加额外 durable hop；不把 runtime path 退化成跨层串行大对象  
**Constraints**: 保持 SQLite-first、单 writer、两阶段执行、outbox-after-commit、workspace 安全边界、现有外部契约；不引入兼容层、双路径、旁路写库、旁路发送  
**Scale/Scope**: 全仓库后端架构重构，覆盖 `cmd/simiclaw/`、`internal/http/`、`internal/channels/`、`internal/gateway/`、`internal/runtime/`、`internal/runner/`、`internal/outbound/`、`internal/store/`、`internal/query/`、`internal/prompt/`、`internal/memory/`、`internal/workspace*`、`internal/tools/`、`internal/provider/`、`internal/bootstrap/`、`tests/architecture/` 及相关文档

## Constitution Check

- `PASS`: 仍以 SQLite 为唯一 runtime facts 来源，不引入额外 durable state owner。
- `PASS`: 仍保持 claim -> execute -> finalize 两阶段执行和 outbox-after-commit。
- `PASS`: 新边界会以显式 use-case seam、consumer-owned interface 和 architecture tests 固化，而不是靠口头约定。
- `PASS`: workspace、memory、prompt 仍停留在明确的安全边界内，不让 Runtime 直接走 ad hoc filesystem access。
- `PASS`: 计划采用可回滚 migration slices，每个 slice 完成后都要求删掉被替换的旧 owner 路径，不允许双路径长期并存。
- `PASS`: 外部能力通过 Capability Plane 接入，但 durable state 仍只由批准的写路径推进。

## Project Structure

### Documentation (this feature)

```text
specs/003-four-plane-architecture-refactor/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── four-plane-boundaries.md
└── tasks.md
```

### Target Source Code Shape

```text
cmd/
└── simiclaw/
    └── internal/                  # Surface (CLI) adapters only

internal/
├── http/                          # Surface adapters for HTTP ingest/query/stream
├── channels/                      # Surface adapters for Telegram and other channels
├── gateway/                       # Runtime-side command ingress boundary and session/routing policy
├── runtime/
│   ├── kernel/                    # claim / execute / finalize orchestration
│   ├── events/                    # observe publication / replay contracts
│   ├── lanes/                     # session/lane ownership strategy
│   ├── payload/                   # runtime payload dispatch
│   ├── workers/                   # recovery / cron / delivery-related workers
│   └── model/
├── runner/                        # agent execution decomposition owned by Runtime
├── outbound/                      # durable delivery coordination owned by Runtime
├── store/
│   ├── tx/                        # write facts
│   ├── projections/               # derived/session projections
│   └── queries/                   # low-level fact/query adapters
├── query/                         # Context/State read-model service for Surface
├── prompt/                        # Context assembly and system assets
├── memory/                        # explicit memory capability/context owner
├── workspace/
├── workspacefile/                 # workspace safety boundary
├── tools/                         # Capability Plane
├── provider/                      # Capability Plane
├── bootstrap/                     # composition root
└── config/

pkg/
├── api/                           # stable external contract
├── model/                         # shared domain primitives
└── logging/

web/                               # consumes Surface contracts; not a backend owner plane
tests/
└── architecture/
```

**Structure Decision**: 这次重构把“四块”作为 owner map，而不是强制新增四个顶层目录。优先保留已经形成稳定认知的顶层包名，只重写它们的职责边界、依赖方向和中心对象形状；`command/query/observe` 作为显式 seam 存在，但不单独提升为第五个 owner plane。

## Core Control Flow After Refactor

### Command Path

`Surface adapter -> normalized command ingress -> gateway/runtime command boundary -> runtime kernel -> context assembly + capability invocation -> finalize transaction -> outbox/runtime events -> surface-visible response or stream`

### Query Path

`Surface adapter -> query boundary -> projections/read models -> serialized response`

### Observe Path

`Surface adapter -> observe subscription boundary -> runtime events/replay -> transport-specific stream output`

### Streaming Chat Composition Path

`HTTP stream surface -> command boundary -> runtime observe/replay boundary -> SSE frame composition in Surface -> terminal event delivery`

补充规则：

- `POST /v1/chat:stream` 可以继续作为一个对外组合入口存在，但它只允许组合调用 command 和 observe seams，不允许重新拥有 query fallback 或 runtime orchestration。
- CLI / Web / shared client 的 retry、polling fallback 和 stream unsupported recovery 归属客户端 consumer，而不是后端 stream handler。

### Admin / Host Control Path

`CLI or internal host control -> explicit host control boundary -> process-local supervision actions`

规则：

- 只有 durable command 进入批准的写路径。
- 只有 process-local host control 可以绕过 durable command ingestion。
- Surface 不直接 import facts store；Capability 不直接推进 durable state。

## Implementation Slices

### Slice 1 - Plane Map, Contract Freeze, And Guardrails

**Goal**: 先把四块 owner map、target source-code shape、外部契约冻结规则和 use-case boundary 文档化，并让架构测试能够阻止明显越界。

**Likely Files**:

- `ARCHITECTURE.md`
- `docs/design-docs/module-boundaries.md`
- `tests/architecture/*.go`
- `internal/bootstrap/app.go`
- 新增或更新边界 DTO / interface 所在包

**Changes**:

- 产出目标 source-code shape，并把主要现存模块映射到四块 owner。
- 把 target source-code shape 和 current-to-target module mapping 作为后续代码切片启动前的显式 gate。
- 明确 `web/` 只消费 Surface 契约，不归入后端 adapter owner。
- 固定外部契约冻结规则：内部边界可重写，现有 HTTP/SSE/CLI/Web 可见契约默认不变。
- 为当前 `chat:stream` 组合行为产出显式 owner map，明确 command ingest、runtime observe/replay、SSE frame 组装、client fallback 分别归谁。
- 把代表性现有契约测试提升为 mandatory baseline，而不是只写“如有需要再验证”。
- 先补 architecture tests，防止后续改动把 Runtime/Surface/Capability 再次揉回胖对象。

**Verification**:

- `go test ./tests/architecture/... -v`
- `go test ./tests/integration/... -tags=integration -run 'TestIngestToProcessedAndQuerySQLite|TestChatStreamAcceptedToDone|TestRuntimeTracePathExposesClaimExecuteFinalizeAndDelivery' -v`
- `go test ./cmd/simiclaw/internal/client/... -v`
- `make docs-style`
- `make web-ci`

### Slice 2 - Runtime / Capability Core Decomposition

**Goal**: 拆掉当前执行链路中的混合中心对象，把 Runtime 和 Capability 的 owner 明确化。

**Likely Files**:

- `internal/runtime/...`
- `internal/runner/...`
- `internal/outbound/...`
- `internal/gateway/...`
- `internal/tools/...`
- `internal/provider/...`

**Changes**:

- 把 agent execution、payload dispatch、delivery coordination、worker host、runtime observe publication 拆成显式 owner。
- Runtime 通过清晰的 provider/tool/skill invocation boundary 调 Capability，不把选择逻辑、超时与失败语义散落到 Surface 或 store 适配层。
- 本 slice 先固定 Runtime <-> Capability invocation seam 和 owner 责任；Capability Plane 的完整清理、注册点收口和扩展面治理放到后续 US5 完成。
- 删除或收缩同时承担 supervision、execution、transport glue 的胖对象。

**What Stays Fixed**:

- `claim -> execute -> finalize` 和 durable delivery 语义不变。
- `memory_flush`、`compaction`、`cron_fire` 仍走 `RunModeNoReply`。
- 外部请求入口暂不改协议。

**Verification**:

- `go test ./tests/architecture/... -v`
- `make test-unit`
- `make test-unit-race-core`
- `make accept-current`

### Slice 3 - Context / State Separation

**Goal**: 把事实写入、读模型查询、prompt/context 组装与 workspace 安全边界分开，消除 god repository 形状。

**Likely Files**:

- `internal/store/...`
- `internal/query/...`
- `internal/prompt/...`
- `internal/memory/...`
- `internal/workspace/...`
- `internal/workspacefile/...`

**Changes**:

- Runtime 只通过写事实 contract 推进状态。
- Surface 查询只通过 query/projection contract 读取状态。
- prompt/memory/workspace/context 只通过只读上下文组装边界供 Runtime 消费。
- 删除把 facts、queries、workspace access 混在一起的仓库接口或 service。

**Verification**:

- `go test ./tests/architecture/... -v`
- `make test-unit`
- `make accept-current`

### Slice 4 - Surface Adapter Convergence

**Goal**: 让 HTTP、CLI、Telegram、inspect、streaming 全部只消费显式 command/query/observe seams，不再把核心行为定义在 adapter 里。

**Likely Files**:

- `cmd/simiclaw/internal/...`
- `internal/http/...`
- `internal/channels/...`
- `web/` only if consumer tests or contract fixtures need update

**Changes**:

- 把 ingest、query、observe、host control 路径在 Surface 明确分开。
- 拆除同时承担 command ingest、query fallback、runtime observe、stream output 的单一入口对象。
- 明确组合流式入口的新 owner 分工：`internal/http/stream` 只负责编排 command + observe 的表层组合和 SSE framing；terminal query fallback 不回流到后端 stream handler，而归 CLI/Web/shared client consumer。
- `web/` 只按 contract consumer 校验，不承担后端 owner 迁移。

**Verification**:

- `go test ./tests/architecture/... -v`
- `go test ./tests/integration/... -tags=integration -run 'TestIngestToProcessedAndQuerySQLite|TestChatStreamAcceptedToDone|TestRuntimeTracePathExposesClaimExecuteFinalizeAndDelivery' -v`
- `make test-unit`
- `make accept-current`
- `go test ./cmd/simiclaw/internal/client/... -v`
- `make web-ci`

### Slice 5 - Bootstrap, Cleanup, And Governance Closure

**Goal**: 收掉临时 glue、更新装配和文档，确保四块骨架在仓库里可持续存在。

**Likely Files**:

- `internal/bootstrap/app.go`
- `ARCHITECTURE.md`
- `docs/design-docs/*.md`
- `tests/architecture/*.go`

**Changes**:

- `bootstrap` 只保留 composition root 责任，不再承担隐式业务编排。
- 删除被新 owner 替代的旧中心对象、过渡 import 和灰色 supporting packages。
- 把四块边界、contract freeze 和 validation commands 写进文档与测试。

**Verification**:

- `go test ./tests/architecture/... -v`
- `make test-unit`
- `make test-unit-race-core`
- `make accept-current`
- `make docs-style`

## Migration Rules

- 每个 slice 必须在同一变更里完成“新 owner 接管 + 旧路径删除或降级为纯测试辅助”，不允许长期双路径并存。
- 任何 accepted slice 都必须保持仓库可构建、可测试、可回滚。
- 如果某个 slice 需要修改外部契约或产品能力集合，必须停下并回到新 spec，而不是在当前计划内扩 scope。
- 优先改架构测试和 boundary contracts，再改实现；不先改 guardrails 的 slice 不允许进入大规模代码重组。

## Validation Strategy

- owner、边界、文档变化：至少运行 `go test ./tests/architecture/... -v` 和 `make docs-style`。
- 常规后端行为变化：至少运行 `make test-unit`。
- runtime、store、ingest、outbound、prompt、workspace 语义变化：至少运行 `make accept-current`。
- goroutine、lane、worker lifecycle 或 supervision 变化：至少运行 `make test-unit-race-core`。
- 触碰 command/query/observe seam、`/v1/events:ingest`、`/v1/chat:stream`、CLI stream fallback 或 web stream consumption 的改动，至少运行 `go test ./tests/integration/... -tags=integration -run 'TestIngestToProcessedAndQuerySQLite|TestChatStreamAcceptedToDone|TestRuntimeTracePathExposesClaimExecuteFinalizeAndDelivery' -v`、`go test ./cmd/simiclaw/internal/client/... -v`，以及 `make web-ci`。

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| 把四块 owner map 做成“新名字包装旧混合逻辑” | 高 | Slice 1 先补 architecture tests 和 target source-code shape；没有 guardrail 不进入大改 |
| 把 `web/` 错当成后端 Surface adapter 导致职责判断失真 | 中 | 在 research 和 contracts 里明确 `web/` 只是 consumer，不承担后端 owner |
| 只拆 Runtime，不冻结外部契约，导致 refactor 顺手改 wire behavior | 高 | 默认冻结现有 HTTP/SSE/CLI/Web 可见契约；若要改，必须新开 spec |
| `chat:stream` 这类组合语义拆分后没有明确 owner，导致行为回归或新中心对象回流 | 高 | 先产出 command ingest / runtime observe / SSE framing / client fallback 的显式 owner map，并把代表性 contract tests 提升为必跑项 |
| 为了“更纯”引入新的 god use-case package | 中 | use-case boundary 只作为 seam，不额外提升成第五个 owner plane |
| facts/projections/context 拆分不彻底，留下大仓库接口 | 高 | Slice 3 以删除混合 repository 为完成标准，并由 architecture tests 固化 |

## Complexity Tracking

本计划不接受额外复杂度豁免。

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| None | N/A | N/A |
