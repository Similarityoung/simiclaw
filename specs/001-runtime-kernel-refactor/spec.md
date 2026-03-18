# Feature Specification: 重构当前项目为可扩展 Agent Runtime 内核

**Feature Branch**: `codex/001-runtime-kernel-refactor` (`speckit` feature id: `001-runtime-kernel-refactor`)  
**Created**: 2026-03-18  
**Status**: Draft  
**Input**: User description: "/speckit.specify 帮我重构目前的项目"

## Clarified Decisions

- 本次重构的强保边界仅包括 SQLite 事实模型与核心运行不变量；除这些边界外，现有后端实现、目录组织、适配层和接口形状都允许重构或重写。
- 第一阶段优先重建 runtime kernel 和扩展接口，不把当前 HTTP、CLI、Telegram 接入实现作为首要保留目标；这些能力应在新内核稳定后重新挂接。
- 规划中的默认实施顺序固定为：`runtime kernel -> gateway/routing -> http/channels/delivery -> concurrency lanes`。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 维护者能够在不破坏运行语义的前提下演进内核 (Priority: P1)

作为项目维护者，我需要把当前后端重构成更清晰的内核结构，这样我后续推进 agent loop、channels、delivery、resilience 和 concurrency 时，不必继续把逻辑堆进少数几个中心文件。

**Why this priority**: 当前最大的风险不是功能缺失，而是继续在现有形状上加功能会让代码越来越难维护，最终阻塞所有后续能力开发。

**Independent Test**: 只实现这一故事时，团队就应该能在不改变数据库事实模型和核心运行语义的前提下，重新组织 runtime/gateway/http/channels/delivery 的扩展点，并用新的内核结构承接后续能力开发。

**Acceptance Scenarios**:

1. **Given** 当前 SQLite-first、两阶段执行和 outbox-after-commit 语义已经存在，**When** 维护者阅读新的内核结构并实现一次局部重构，**Then** 事件处理主链路仍保持相同的持久化与执行语义，即使适配层和接口形状已经重组。
2. **Given** 新增一种内核扩展需求，例如 payload handler 或 worker role，**When** 维护者按新的内核结构接入，**Then** 不需要继续修改多个中心文件来完成最小接入。

---

### User Story 2 - 扩展开发者能够通过显式扩展点接入新能力 (Priority: P1)

作为扩展开发者，我希望新增 channel、payload type、worker、delivery policy 时，有明确的注册点和约束边界，而不是依赖现有分支散落的位置。

**Why this priority**: 这是后续 `s02` 到 `s10` 能否持续落地的关键。如果没有清晰扩展点，任何新能力都会演变成高风险的跨层修改。

**Independent Test**: 只实现这一故事时，团队应该能用一个最小的 mock 扩展证明新结构确实支持“加一个能力而不是改一串旧逻辑”，并且这些扩展是建立在新的接口契约之上。

**Acceptance Scenarios**:

1. **Given** 一个新的 channel integration 需求，**When** 开发者按统一 ingress/egress 契约接入，**Then** 不需要直接依赖 store 或修改运行语义核心事务，并且旧 channel 实现是否保留不构成设计前提。
2. **Given** 一个新的 payload handler 或 worker role 需求，**When** 开发者按约定注册，**Then** 接入点应当是明确命名且文档化的边界，而不是隐藏在中心分支逻辑中。

---

### User Story 3 - 运行维护者能够清晰追踪数据流、恢复路径与后台工作 (Priority: P2)

作为运行维护者，我需要能快速看清一条 event 从 ingest 到 finalize 再到 delivery 的路径，以及后台 worker、cron、retry 和 recovery 分别属于哪里。

**Why this priority**: 当前系统的宏观架构存在，但微观表达不够清晰，导致排障、评审和后续重构成本偏高。

**Independent Test**: 只实现这一故事时，团队应该能用文档和代码入口快速定位 ingest、execution、finalize、delivery、recovery、scheduling 各自的 owner 与边界。

**Acceptance Scenarios**:

1. **Given** 一次 event 处理异常，**When** 维护者沿着文档和代码入口追踪，**Then** 可以明确定位它是在 ingress、claim、runner、finalize 还是 outbox/delivery 阶段失败。
2. **Given** 一个后台 worker 行为变更，**When** 维护者检查新的内核结构，**Then** 可以单独定位该 worker 的心跳、租约、重试与停止语义。

---

### User Story 4 - 产品演进能够在保留现有事实模型的前提下继续推进 (Priority: P3)

作为产品和平台负责人，我希望在不推倒 SQLite 事实模型和外部运行语义的前提下，逐步把当前项目推进成可以承载 channel、routing、intelligence、delivery、resilience 和 concurrency 的平台。

**Why this priority**: 全量重写会重新踩一遍幂等、恢复、持久化和交付语义的坑；渐进式重构可以在保留现有资产的同时建立未来能力的内核。

**Independent Test**: 只实现这一故事时，团队应该能明确哪些部分保留、哪些部分重构、哪些部分局部重写，并以阶段性迁移方式推进。

**Acceptance Scenarios**:

1. **Given** 未来能力规划已经明确，**When** 团队按照重构 spec 推进，**Then** 新能力开发应当建立在稳定内核之上，而不是继续叠加临时实现。
2. **Given** 需要分阶段迁移，**When** 团队执行每一阶段，**Then** 每一步都应存在可验证、可回滚的中间状态。

### Edge Cases

- 重构进行到一半时，旧实现和新实现并存，如何避免运行语义出现双写、旁路发送或重复执行，同时允许旧适配层被临时下线或替换。
- 旧 payload type、旧 channel、旧 outbox 行为在迁移期内仍需保持兼容，如何定义“兼容”的边界。
- 引入未来 lane / session serialization 模型时，如何避免破坏现有单 writer 与两阶段执行约束。
- 文档、测试和实现不同步时，如何防止新的内核结构再次退化成“代码能跑但边界失真”。
- `.specify/` 工作流和仓库自身 `codex/` 分支规范并存时，如何保持 feature spec 与实际开发分支一致。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST preserve the current SQLite-first runtime fact model and the persisted meanings of `events`, `runs`, `messages`, `outbox`, `scheduled_jobs`, `heartbeats`, and derived `sessions`.
- **FR-002**: System MUST preserve the current two-phase runtime semantics: claim and create `runs(started)` in a transaction, execute outside the transaction, then finalize results in one result transaction.
- **FR-003**: System MUST preserve outbox-after-commit semantics for all real outbound delivery.
- **FR-004**: System MUST reorganize the backend around explicit extension points for payload handling, external ingress/egress, background workers, delivery senders, and resilience policies.
- **FR-005**: System MUST ensure all HTTP and channel ingress normalizes into a common ingest request shape before entering gateway.
- **FR-006**: System MUST ensure all channel egress flows through durable outbox intent and channel-specific sender selection rather than direct post-run sends.
- **FR-007**: System MUST separate runtime responsibilities into clearly owned areas for ingress normalization, claim/execution, finalize, delivery, recovery, scheduling, and runtime event publication.
- **FR-008**: System MUST maintain session isolation and context policy as explicit, documented behavior, including overflow handling, memory boundaries, and no-reply payload rules.
- **FR-009**: System MUST support staged migration so that the refactor can be delivered in reversible slices rather than requiring an all-at-once rewrite.
- **FR-010**: System MUST provide developer-facing architecture guidance for the new kernel shape, including where to add new handlers, workers, channels, and delivery logic.
- **FR-011**: System MUST keep package and model boundaries aligned with the project constitution, especially the distinction between `pkg/api`, `pkg/model`, and `internal/<subsystem>/model`.
- **FR-012**: System MUST leave room for future session-serialized or lane-based concurrency without requiring another top-level rewrite of runtime ownership and worker coordination.
- **FR-013**: System MUST treat current HTTP, CLI, Telegram, and other external boundaries as replaceable layers; preserving their current implementation shape is NOT a requirement for the first migration slice.
- **FR-014**: System MUST define the new kernel contracts before rebuilding HTTP/channels boundaries, including contracts for runtime execution, payload handling, external ingress/egress, worker lifecycle, and delivery senders.
- **FR-015**: System MUST sequence implementation work so that runtime kernel and extension interfaces are established before gateway/routing, HTTP/channel/delivery, and concurrency-lane refactors.

### Key Entities *(include if feature involves data)*

- **Runtime Kernel**: The explicitly owned core that coordinates claim, execution, finalize, delivery, recovery, and scheduling without changing persisted runtime semantics.
- **Extension Point**: A named registration or contract surface where new payload handlers, HTTP/channel integrations, worker roles, delivery senders, or resilience policies can be added.
- **Channel Pipeline**: The normalized ingress plus durable egress flow for HTTP, CLI, and Telegram.
- **Execution Lane**: The future-facing concurrency unit used to reason about session ordering, named queue behavior, and bounded worker ownership.
- **Migration Slice**: A staged, reversible refactor step that changes internal structure while preserving current runtime facts and externally observable semantics.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For each accepted migration slice, the preserved invariants for SQLite facts, claim/execute/finalize semantics, and durable delivery continue to pass the repository's required validation targets.
- **SC-002**: A maintainer can add one minimal mock extension in each of these categories without modifying core transaction semantics: payload handler, HTTP or channel integration, worker role, and delivery sender.
- **SC-003**: The primary event path from ingest to finalize to delivery can be traced through documented code ownership points without relying on hidden tribal knowledge.
- **SC-004**: No single migration slice requires an all-at-once cutover; each accepted slice can be validated and, if needed, rolled back independently.
- **SC-005**: The resulting architecture and governance docs clearly explain where future work for agent loop, tool use, sessions/context, channels, gateway/routing, intelligence, heartbeat/cron, delivery, resilience, and concurrency should be implemented, starting from the new kernel contracts.
