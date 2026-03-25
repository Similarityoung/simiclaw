# Feature Specification: 按四块统一骨架重建 SimiClaw

**Feature Branch**: `codex/003-four-plane-architecture-refactor` (`speckit` feature id: `003-four-plane-architecture-refactor`)  
**Created**: 2026-03-23  
**Status**: Draft  
**Input**: User description: "/speckit.specify"

## Clarified Decisions

- 本次目标不是继续在现有包结构上做局部收口，而是在保留核心运行不变量的前提下，按统一骨架重写模块边界、调用关系和代码 owner。
- 目标宏观架构固定为四块：`Surface`、`Runtime`、`Context/State`、`Capability Plane`。`command/query/observe` 作为显式用例边界存在，但不再额外上升为第五个顶层平面。
- 必须保留的强约束只有：SQLite-first 事实模型、两阶段执行、outbox-after-commit、`memory_flush|compaction|cron_fire -> RunModeNoReply`、workspace 安全边界。
- 当前内部包路径、装配方式、中心对象形状和混合接口都允许重写；本次不以兼容现有内部目录结构、旧 service 形状或 `chat:stream` 的现有混合实现为目标。
- 默认保留现有产品能力集合：HTTP API、流式会话、inspect、可选 Telegram channel、workspace/memory/tools、后台 worker 与定时任务；但这些能力允许在新的表层接口和内部边界上重新落位。
- `web/` 在本次重构中被视为消费 HTTP/query/stream Surface 契约的客户端，而不是后端 runtime 内的 Surface adapter owner；仅当其消费契约或 fixture 受影响时才需要联动调整。
- 本次默认冻结现有 HTTP、SSE、CLI 与 Web 可观察契约；若需要修改 `/v1/events:ingest`、`/v1/chat:stream`、inspect 输出、CLI 流式行为或前端消费协议，必须另开 spec。
- 本次不是“文件搬家”规格，而是“职责边界 + 控制流 + 状态边界”重写规格；目录变化只是最终结果，不是目标本身。
- 现有 `001-runtime-kernel-refactor` 记录的是上一阶段 kernel/owner 收口事实；本 spec 面向新的全量统一重构目标，不推翻其中已验证的不变量。

## Impacted Areas

- `internal/http/`, `cmd/simiclaw/`, `internal/channels/telegram`: 重新收敛为后端 `Surface` adapters。
- `web/`: 作为 Surface contract consumer 保持联动验证，但不承担后端 adapter owner 职责。
- `internal/gateway/`, `internal/runtime/`, `internal/runner/`, `internal/outbound/`: 重组为 `Runtime` 核心与显式执行 owner。
- `internal/store/`, `internal/query/`, `internal/prompt/`, `internal/memory/`, `internal/workspace/`, `internal/workspacefile/`: 明确区分 facts、projections、workspace context 与 memory/state。
- `internal/tools/`, `internal/provider/` 以及未来的 skills/MCP/model router: 收敛为 `Capability Plane`，不再承担 durable state owner。
- `internal/bootstrap/app.go`: 从“直接拼装混合 service”重构为基于新边界的装配入口。
- 架构测试、设计文档与 speckit 计划文档：必须同步更新，防止新边界再次退化。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 维护者能够按统一骨架理解和演进系统 (Priority: P1)

作为项目维护者，我需要 SimiClaw 具备稳定统一的架构骨架，这样我在新增能力、改执行链路或做 code review 时，不必再先猜“这段逻辑究竟属于 HTTP、runtime、query、workspace 还是 provider”。

**Why this priority**: 当前最大的技术债不是单点 bug，而是职责边界不统一导致的持续扩散；如果不先统一骨架，后续任何功能都会继续堆进少数中心文件。

**Independent Test**: 只实现这一故事时，团队就应该能根据新结构快速回答“一个模块属于哪一块、它能依赖谁、它不能拥有谁的状态”，并通过架构测试阻止明显越界。

**Acceptance Scenarios**:

1. **Given** 维护者需要修改一次 event 从 ingest 到 finalize 的链路，**When** 他沿着新架构定位 owner，**Then** 可以明确区分 Surface、Runtime、Context/State、Capability 各自负责的阶段，而不需要在混合 handler 或胖 service 里逆向推理。
2. **Given** 一个模块同时触碰 transport、执行、查询回放和状态组装，**When** 维护者按新规则审查，**Then** 该模块会被判定为职责越界，而不是继续作为“方便实现”的中心对象存在。

---

### User Story 2 - 适配层开发者能够新增或替换外部入口而不改内核 (Priority: P1)

作为接入层开发者，我希望新增一个 Surface adapter 或替换一个旧入口时，只需要面向显式 command/query/observe 用例边界，而不必直接触达 runtime/store/provider 的内部细节。

**Why this priority**: 这是把 Web/CLI/HTTP/Telegram 从“定义系统行为的地方”降级为“消费统一用例的外壳”的关键。

**Independent Test**: 只实现这一故事时，团队应该能新增一个最小 adapter 或重写一个旧 adapter，而无需修改 runtime 核心事务语义、store 事实层或 capability 注册机制。

**Acceptance Scenarios**:

1. **Given** 一个新的外部入口需求，**When** 开发者接入该入口，**Then** 它只通过显式用例接口提交 command、读取 projection 或订阅 observe，不直接依赖事实存储或执行内核。
2. **Given** 一个旧入口需要被替换，**When** 替换完成，**Then** runtime 的 claim/execute/finalize 语义、facts 模型和 capability 接口不需要因此同步重写。

---

### User Story 3 - Runtime 开发者能够演进执行内核而不夹带表层和能力层逻辑 (Priority: P1)

作为 runtime 开发者，我希望调度、kernel、agent loop、worker lifecycle 和 durable delivery 有明确 owner，这样我演进执行模型时，不会顺手把 HTTP/SSE、prompt 文件读取或 provider 选择继续塞进同一个胖对象里。

**Why this priority**: 当前 `chat:stream`、`Supervisor`、`Runner` 等位置已经暴露出跨层混合问题；如果内核 owner 不先收紧，重构后仍会重新长成新的中心文件。

**Independent Test**: 只实现这一故事时，团队应该能针对 loop/kernel/agent executor/worker 分别进行重构，并保持 SQLite-first 和两阶段执行语义不变。

**Acceptance Scenarios**:

1. **Given** 需要重构 agent 执行链路，**When** 开发者实现新的 runtime 结构，**Then** provider 调用、tool loop、payload plan、memory write、worker 调度等职责应被显式分解，而不是继续挤在单一 `Runner` 中。
2. **Given** 需要新增一种后台 worker 或调度策略，**When** 开发者按新结构接入，**Then** 只需扩展 Runtime 内核的注册点与 facts 契约，而不必修改 Surface adapter 或 capability provider 的装配细节。

---

### User Story 4 - 状态与上下文 owner 能够被清晰区分 (Priority: P2)

作为维护者，我需要明确哪些是 durable facts，哪些是 projections，哪些是 workspace/memory/prompt context，这样系统不会再次把 SQLite 状态机、workspace 文件和提示词上下文混成一个大而全的“状态层”。

**Why this priority**: 对 SimiClaw 而言，真正的主模型是 SQLite facts；如果不把 facts/projections/context 区分清楚，四块架构很快又会退化成泛化的“state everything”。

**Independent Test**: 只实现这一故事时，团队应能把一次状态读取、一次上下文装配和一次 projection 查询分别落到不同 owner 上，并证明它们不共享模糊的仓库抽象。

**Acceptance Scenarios**:

1. **Given** 一次 runtime 执行需要读取 event/run/message facts 并更新结果，**When** 代码实现该流程，**Then** 它依赖的是 facts 契约，而不是混合了 query/context/workspace 逻辑的通用 store service。
2. **Given** 一次 prompt/context 组装需要读取 workspace、memory 和派生查询结果，**When** 代码实现该流程，**Then** 它不会直接拥有 runtime state transition，而是只消费 context/state owner 暴露的只读能力。

---

### User Story 5 - 平台能力扩展能够通过 Capability Plane 接入 (Priority: P2)

作为扩展开发者，我希望 MCP、skills、tools、模型供应商和模型路由都被统一视为 capability source，而不是半个 runtime、半个状态层的混合 owner。

**Why this priority**: 当前 provider/tools 已经是半显式扩展点，但未来如果引入 skills/MCP/router，没有统一 capability plane 很容易再次把外部能力耦合回业务主链路。

**Independent Test**: 只实现这一故事时，团队应该能新增一个 capability provider 或工具能力，而不需要修改事实模型、adapter 协议或执行事务结构。

**Acceptance Scenarios**:

1. **Given** 需要接入新的模型供应商、模型路由或 MCP 能力，**When** 开发者按新结构扩展，**Then** 该改动应停留在 capability plane 和 runtime 调用边界之内。
2. **Given** 某个 capability 超时、失败或被策略拒绝，**When** runtime 处理该结果，**Then** 失败语义通过显式错误与 trace/log 传播，而不是让 capability 层私自推进 durable state。

### Edge Cases

- 全量重构过程中如何在不引入兼容层、双写或旁路写入的前提下完成切换，并保证任一阶段都仍符合 SQLite-first 与两阶段执行约束。
- 旧的混合接口（尤其是同时承担 ingest、stream、query fallback 的入口）被拆分后，如何保持用户能力可恢复，而不是把“暂时没接回”误当作可接受状态。
- 当前 `chat:stream` 这类组合入口拆分后，如何把 command ingest、runtime observe/replay、服务端 stream frame 组装、客户端 retry/poll fallback 分别归到显式 owner，而不是换个位置继续混在一个对象里。
- 把 facts、projections 和 context owner 分开后，如何避免重新出现“一个仓库接口什么都能读、什么都能写”的退化设计。
- Capability plane 接入外部 provider、MCP 或技能时，如何保证超时、重试、权限边界和 workspace 安全仍停留在正确边界。
- Admin 类动作中，哪些属于 durable command，哪些属于进程内 runtime host control，必须在规格中明确，避免直接拍 `EventLoop` 破坏状态机约束。
- 重构后如果仍存在根级灰色 supporting packages，如何通过架构测试和文档阻止再次回流。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST reorganize the backend around four top-level ownership planes: `Surface`, `Runtime`, `Context/State`, and `Capability Plane`.
- **FR-002**: System MUST preserve the current SQLite-first runtime fact model and the persisted meanings of `events`, `runs`, `messages`, `outbox`, `scheduled_jobs`, `heartbeats`, and derived `sessions`.
- **FR-003**: System MUST preserve the current two-phase runtime semantics: claim and create `runs(started)` in a transaction, execute outside the transaction, then finalize results in one result transaction.
- **FR-004**: System MUST preserve outbox-after-commit semantics for all real outbound delivery.
- **FR-005**: System MUST preserve the current `RunModeNoReply` semantics for `memory_flush`, `compaction`, and `cron_fire`.
- **FR-006**: Surface adapters MUST be replaceable shells responsible only for transport concerns such as auth, request normalization, serialization, and streaming protocol output.
- **FR-007**: Surface adapters MUST NOT directly own runtime orchestration, state transitions, query fallback composition, or capability selection logic.
- **FR-008**: System MUST expose explicit command, query, and observe use-case boundaries for Surface adapters to call, even if these use cases are not modeled as a separate top-level plane in repository diagrams.
- **FR-009**: Runtime MUST own event scheduling, lane/session serialization strategy, kernel claim/execute/finalize orchestration, agent execution, background workers, recovery, and durable delivery coordination.
- **FR-010**: Runtime MUST NOT own HTTP/SSE/CLI/Telegram protocol formatting or other transport-specific behavior.
- **FR-011**: Context/State MUST distinguish among durable facts, projections/read models, and workspace/memory/agent context instead of collapsing them into one generic store owner.
- **FR-012**: Durable facts MUST remain centered on SQLite; workspace files, memory files, prompt assets, and agent specs MUST NOT become alternate runtime fact sources.
- **FR-013**: Capability Plane MUST own tools, skills, MCP clients, model providers, and model routing as callable capabilities, and MUST NOT directly mutate durable runtime state outside approved write paths.
- **FR-014**: System MUST replace current mixed-center objects with smaller owners, especially for streaming chat entrypoints, runtime host/supervision, and agent execution flow.
- **FR-015**: A single surface module MUST NOT simultaneously own durable command ingest, runtime event observation, terminal query fallback, and protocol streaming output.
- **FR-016**: Admin mutations that change durable state MUST be modeled as explicit commands entering the approved write path; only process-local runtime host control MAY bypass durable command ingestion.
- **FR-017**: Workspace reads and writes MUST continue to flow through explicit workspace safety boundaries; the refactor MUST NOT reintroduce ad hoc filesystem access as a shortcut.
- **FR-018**: System MUST define explicit interfaces between Runtime and Context/State for facts, projections, and context assembly, and MUST avoid “god repository” interfaces that mix write facts, read projections, and workspace access.
- **FR-019**: System MUST define explicit interfaces between Runtime and Capability Plane for provider execution, tool invocation, skill access, and future MCP integration, with timeout and failure semantics owned at the boundary.
- **FR-020**: System MUST keep package and model boundaries aligned with the constitution, especially the distinction among `pkg/api`, `pkg/model`, and `internal/<subsystem>/model`.
- **FR-021**: System MUST permit a full internal rewrite without preserving current package layout or mixed service shapes, while still restoring the current product capability set before the refactor is considered complete.
- **FR-022**: System MUST update architecture tests, design docs, and migration guidance so the four-plane ownership model remains enforceable after the rewrite.
- **FR-023**: System MUST eliminate root-level grey packages or ambiguous owners that do not clearly belong to one of the four planes.
- **FR-024**: System MUST provide a documented target source-code shape that maps current major subsystems into the new four-plane ownership model before implementation begins.
- **FR-025**: System MUST preserve the current externally observable HTTP, SSE, CLI, and web-consumed contracts throughout this refactor unless a separate approved spec explicitly changes them.
- **FR-026**: System MUST decompose current mixed `chat:stream`-style behavior into explicit owners for command ingest, runtime observe/replay, server-side stream framing, and client-side retry or polling fallback, without dropping any currently supported visible behavior.

### Key Entities *(include if feature involves data)*

- **Surface Adapter**: 对外暴露系统能力的入口或出口壳层，例如 HTTP、CLI、Telegram 和服务端流式传输端点。
- **Use-Case Boundary**: 供 Surface 调用的显式 command、query、observe 编排接口，本身不拥有 transport 协议，也不拥有底层执行事务。
- **Runtime Kernel**: 负责 claim、execute、finalize、worker lifecycle、recovery、delivery coordination 的执行内核。
- **Durable Fact**: 存放在 SQLite 中、可恢复可审计的运行时事实，例如 events、runs、messages、outbox 和 jobs。
- **Projection / Read Model**: 基于 durable facts 派生的查询视图和缓存，例如 sessions、inspect/query 读模型和终态回放所需记录。
- **Context Asset**: prompt、workspace、memory、agent spec 等用于运行时上下文组装的状态或文件资源，但它们不是 runtime facts 的替代来源。
- **Capability Source**: 被 Runtime 调用的外部能力源，例如 tool、skill、MCP、provider 和 model router。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 新架构文档和代码能把每个主要模块明确映射到 `Surface`、`Runtime`、`Context/State`、`Capability Plane` 之一，不再存在关键模块横跨多个 plane 且 owner 不清的情况。
- **SC-002**: 一名维护者可以新增一个最小 Surface adapter，而无需修改 runtime 事务语义、SQLite facts 或 capability 注册机制。
- **SC-003**: 一名维护者可以新增一个最小 capability provider 或工具能力，而无需修改 Surface adapter 或 durable fact schema。
- **SC-004**: 针对一次代表性的 event 路径，维护者能够清晰追踪 `command ingest -> durable facts -> runtime execution -> projections/observe -> delivery` 的 owner 与代码入口。
- **SC-005**: 代表性架构测试能够阻止至少以下几类退化：Surface 直接依赖 facts 存储实现、Runtime 直接拥有 transport 协议逻辑、Capability Plane 直接推进 durable state。
- **SC-006**: 重构完成后，不再存在同时承担 ingest、query fallback、runtime observe 和 streaming protocol 的单一中心入口对象。
- **SC-007**: 重构完成后，不再存在同时承担 runtime host、worker registry、cron coordination、readiness aggregation 的单一胖对象。
- **SC-008**: 在任一接受的迁移切片完成后，仓库的必需验证仍能证明 SQLite-first、两阶段执行和 durable delivery 语义未被破坏。
- **SC-009**: 代表性现有契约验证继续通过且无需 wire contract 变更，至少覆盖 `/v1/events:ingest`、`/v1/chat:stream`、CLI 流式 fallback 以及 web 对流式契约的消费路径。
