# Tasks: 按四块统一骨架重建 SimiClaw

**Input**: Design documents from `/specs/003-four-plane-architecture-refactor/`
**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/four-plane-boundaries.md`, `quickstart.md`

**Tests**: 本特性明确要求架构守护、现有外部契约基线、集成验证、单元测试、race 验证与 acceptance 回归，因此任务中包含对应测试与验证项。

**Organization**: 任务按共享基础与用户故事分组，保证每个故事都能独立实现、独立验证，并能按迁移切片渐进交付。

## Format: `[ID] [P?] [Story] Description`

- **[P]**: 可以并行执行，前提是修改文件不冲突
- **[Story]**: 对应的用户故事，例如 `US1`
- 每条任务都包含明确文件路径，避免“大而空”的重构项

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 建立四块统一骨架的文档落点、目标源码形状、守护测试落点和实现入口清单

- [ ] T001 创建四块架构总览文档并先写入 target source shape、current-to-target module mapping 与 owner map，同时接入索引：`docs/design-docs/four-plane-architecture.md`、`docs/design-docs/index.md`、`docs/index.md`
- [ ] T002 [P] 创建四块架构守护测试骨架：`tests/architecture/four_plane_boundaries_test.go`
- [ ] T003 [P] 在 `specs/003-four-plane-architecture-refactor/quickstart.md` 中固定 Phase 2 基线 gate、契约敏感路径验证命令与 checkpoint，用作后续每个 slice 的统一验收入口

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 固定所有用户故事共享的 seam、guardrails、target source shape 使用约束与外部契约基线；本阶段完成前不要进入故事实现

**⚠️ CRITICAL**: 本阶段完成前，不应进入任何用户故事实现

- [ ] T004 在 `internal/gateway/contracts.go`、`internal/runtime/kernel/contracts.go`、`internal/query/service.go`、`internal/runtime/events/hub.go` 中定义显式 `command/query/observe` seam 与 host-control 边界
- [ ] T005 [P] 在 `internal/runtime/kernel/facts.go`、`internal/runtime/kernel/events.go`、`internal/provider/types.go`、`internal/tools/types.go`、`internal/runner/model/types.go` 中固定 Runtime -> Context/State 与 Runtime -> Capability 的 consumer-owned contract
- [ ] T006 [P] 在 `tests/architecture/boundaries_test.go`、`tests/architecture/http_channel_boundaries_test.go`、`tests/architecture/runtime_kernel_boundaries_test.go`、`tests/architecture/owner_closure_test.go`、`tests/architecture/four_plane_boundaries_test.go` 中编码四块 owner guardrails，禁止新的混合中心对象回流，并运行 `go test ./tests/architecture/... -v`、`make docs-style` 固定文档/边界基线
- [ ] T007 [P] 在 `tests/integration/runtime_integration_test.go`、`tests/integration/runtime_trace_path_test.go`、`cmd/simiclaw/internal/client/client_test.go`、`web/src/app/router/router.test.tsx` 中固定 `/v1/events:ingest`、`/v1/chat:stream`、CLI stream fallback、web stream consumption 的外部契约基线，并运行 `go test ./tests/integration/... -tags=integration -run 'TestIngestToProcessedAndQuerySQLite|TestChatStreamAcceptedToDone|TestRuntimeTracePathExposesClaimExecuteFinalizeAndDelivery' -v`、`go test ./cmd/simiclaw/internal/client/... -v`、`make web-ci` 作为 Phase 2 contract gate
- [ ] T008 更新 `internal/bootstrap/app.go` 与 `internal/http/server.go`，让应用装配和 HTTP server 依赖 Phase 2 中定义的 seam，而不是继续隐式拼装混合 service

**Checkpoint**: 四块 seam、target source shape、契约基线和架构守护已经固定且基线验证通过，后续故事可以在不改变外部契约的前提下推进

---

## Phase 3: User Story 1 - 维护者能够按统一骨架理解和演进系统 (Priority: P1) 🎯 MVP

**Goal**: 让维护者能够直接从文档、包注释和架构测试中定位 owner，而不再依赖口头上下文

**Independent Test**: 完成本故事后，维护者应能从仓库中明确回答任一核心模块属于哪一块、允许依赖谁、不能拥有谁的状态，并由架构测试阻止明显越界

### Tests for User Story 1

- [X] T009 [P] [US1] 在 `tests/architecture/four_plane_boundaries_test.go` 与 `tests/architecture/owner_closure_test.go` 中补 module-to-plane 映射断言与“禁止单模块同时承担 transport + execution + observe + fallback”断言
- [X] T010 [P] [US1] 在 `tests/architecture/boundaries_test.go` 与 `tests/architecture/runtime_kernel_boundaries_test.go` 中补四块依赖方向断言，覆盖 `Surface -> Runtime/Query`、`Runtime -> Context/State`、`Runtime -> Capability`

### Implementation for User Story 1

- [X] T011 [US1] 更新 `ARCHITECTURE.md`、`docs/design-docs/four-plane-architecture.md`、`docs/design-docs/module-boundaries.md`，把 Phase 1/2 固定的 owner map、依赖方向和 current-to-target module mapping 同步到仓库长期文档
- [X] T012 [P] [US1] 更新包级 owner 注释与边界说明：`internal/http/stream/doc.go`、`internal/query/model/doc.go`、`internal/runtime/model/doc.go`、`cmd/simiclaw/internal/messages/doc.go`
- [X] T013 [US1] 收敛 composition root 的 owner 说明和 wiring 边界：`internal/bootstrap/app.go`、`cmd/simiclaw/internal/root/command.go`
- [X] T014 [US1] 运行并记录本故事最小验证：`go test ./tests/architecture/... -v`、`make docs-style`，并将结果同步到 `specs/003-four-plane-architecture-refactor/quickstart.md`

**Checkpoint**: User Story 1 完成后，四块统一骨架已经在仓库中可见、可读、可守护

---

## Phase 4: User Story 3 - Runtime 开发者能够演进执行内核而不夹带表层和能力层逻辑 (Priority: P1)

**Goal**: 把执行链路中的混合中心对象拆成明确的 Runtime owner，使 runtime 演进不再携带 transport 和 capability glue

**Independent Test**: 完成本故事后，团队应能重构 loop/kernel/agent executor/worker，而不破坏 SQLite-first、两阶段执行和 durable delivery 语义

### Tests for User Story 3

- [X] T015 [P] [US3] 在 `internal/runtime/kernel/kernel_test.go`、`internal/runtime/eventloop_test.go`、`internal/runtime/kernel/kernel_race_test.go` 中补 claim/execute/finalize、worker lifecycle 与 host control 的边界测试
- [X] T016 [P] [US3] 在 `tests/integration/runtime_kernel_integration_test.go` 与 `tests/integration/runtime_trace_path_test.go` 中补运行时主链路和 observe publication 的集成验证

### Implementation for User Story 3

- [X] T017 [US3] 重构 `internal/runtime/eventloop.go`、`internal/runtime/kernel/{service.go,claim.go,finalize.go}`、`internal/runtime/kernel_adapter.go`，把 claim/execute/finalize orchestration 与 host control 明确收口到 Runtime
- [X] T018 [P] [US3] 重构 `internal/runner/{runner.go,tool_executor.go,prompt_assembler.go,stream_sink.go,trace_assembler.go}`，把 agent execution、tool loop、prompt glue 与 stream sink 责任拆开
- [X] T019 [P] [US3] 重构 `internal/runtime/events/hub.go`、`internal/runtime/stream_sink.go`、`internal/outbound/delivery/worker.go`、`internal/outbound/sender/router.go`，让 runtime observe publication 与 durable delivery coordination 不再依赖混合中心对象
- [X] T020 [US3] 更新 `internal/runtime/host_control.go`、`internal/runtime/readiness.go` 与 `internal/runtime/delivery_intent.go`，把 host control/readiness/worker host 收回 Runtime owner，不再混入 transport 或 capability 细节
- [X] T021 [US3] 运行并记录本故事最小验证：`go test ./internal/runtime/... ./internal/runner/... ./internal/outbound/... -v`、`make test-unit-race-core`、`make accept-current`

**Checkpoint**: User Story 3 完成后，Runtime 开发者可以在不触碰 Surface/Capability 细节的前提下演进执行内核

---

## Phase 5: User Story 2 - 适配层开发者能够新增或替换外部入口而不改内核 (Priority: P1)

**Goal**: 让 HTTP、CLI、Telegram 和 Web 只消费显式 seam，而不再直接定义或混入 runtime 内核行为

**Independent Test**: 完成本故事后，团队应能新增或替换一个最小 Surface adapter，而无需修改 runtime 事务语义、facts 模型或 capability 注册机制

### Tests for User Story 2

- [X] T022 [P] [US2] 在 `internal/http/{ingest/handler_test.go,query/handler_test.go,stream/handler_test.go}` 与 `tests/integration/http_ingress_integration_test.go` 中补 HTTP command/query/observe seam 的契约测试
- [X] T023 [P] [US2] 在 `cmd/simiclaw/internal/client/client_test.go`、`tests/integration/cli_integration_test.go`、`web/src/app/router/router.test.tsx` 中补 CLI/Web 的 stream fallback 与 consumer-only 契约测试
- [X] T024 [P] [US2] 在 `internal/channels/telegram/{normalize_test.go,runtime_test.go,filter_test.go}` 与 `tests/integration/telegram_integration_test.go` 中补 Telegram 归一化与 adapter seam 测试

### Implementation for User Story 2

- [X] T025 [US2] 重构 `internal/http/{ingest/handler.go,query/handler.go,stream/handler.go,server.go}`，让 HTTP 只组合 command/query/observe seams，不再直接拥有 runtime orchestration 或 query fallback
- [X] T026 [P] [US2] 重构 `cmd/simiclaw/internal/{client/client.go,chat/async_cmds.go,inspect/command.go}`，把 CLI 的 retry/poll fallback、stream unsupported recovery 和 inspect query 保持在 client consumer 一侧
- [X] T027 [P] [US2] 重构 `internal/channels/telegram/{normalize.go,runtime.go,filter.go}`，让 Telegram adapter 只负责 transport normalization 和 Surface seam 调用
- [X] T028 [P] [US2] 重构 `web/src/lib/api-client.ts` 与 `web/src/app/router/router.test.tsx`，确保 Web 只消费 HTTP/SSE 契约，不编码后端 owner 假设
- [X] T029 [US2] 运行并记录本故事最小验证：`go test ./internal/http/... ./cmd/simiclaw/internal/... ./internal/channels/telegram/... -v`、`go test ./tests/integration/... -tags=integration -run 'TestIngestToProcessedAndQuerySQLite|TestChatStreamAcceptedToDone|TestRuntimeTracePathExposesClaimExecuteFinalizeAndDelivery' -v`、`go test ./cmd/simiclaw/internal/client/... -v`、`make web-ci`

**Checkpoint**: User Story 2 完成后，Surface 入口可以替换或扩展，而不需要再改 Runtime 内核

---

## Phase 6: User Story 4 - 状态与上下文 owner 能够被清晰区分 (Priority: P2)

**Goal**: 把 facts、projections、context assets、workspace safety 边界分开，消除 god repository 和泛化状态层

**Independent Test**: 完成本故事后，团队应能把事实写入、projection 查询和上下文组装分别落到不同 owner，并证明它们不共享模糊仓库接口

### Tests for User Story 4

- [ ] T030 [P] [US4] 在 `internal/store/tx/runtime_repository_test.go`、`internal/store/tx/repository_behavior_test.go`、`tests/architecture/runtime_kernel_boundaries_test.go` 中补 facts/projections/context 分离测试
- [ ] T031 [P] [US4] 在 `internal/prompt/builder_test.go`、`internal/tools/context_get_test.go`、`internal/workspacefile/context_test.go`、`internal/memory/memory_test.go` 中补只读上下文组装与 workspace 安全边界测试

### Implementation for User Story 4

- [ ] T032 [US4] 重构 `internal/store/tx/{runtime_facts.go,claim_work.go,finalize_run.go,ingest_event.go}` 与 `internal/store/{queries/repository.go,projections/sessions.go}`，明确写事实、读查询、投影缓存的 owner
- [ ] T033 [P] [US4] 重构 `internal/query/{service.go,events.go,runs.go,sessions.go}`，让 Surface 只消费 query/projection contract，而不是混合 store/runtime/context
- [ ] T034 [P] [US4] 重构 `internal/prompt/{builder.go,loader.go,fingerprint.go}` 与 `internal/runner/context/{assembler.go,history.go,memory.go}`，把上下文组装收敛到只读 context bundle
- [ ] T035 [P] [US4] 重构 `internal/memory/{get.go,search.go,writer.go}`、`internal/tools/{context_get.go,memory_get.go,memory_search.go,workspace_file.go}`、`internal/workspacefile/context.go`，确保 workspace/memory/context 访问只经过明确边界
- [ ] T036 [US4] 运行并记录本故事最小验证：`go test ./internal/store/... ./internal/query/... ./internal/prompt/... ./internal/workspacefile/... ./internal/tools/... ./internal/memory/... -v`、`make test-unit`、`make accept-current`

**Checkpoint**: User Story 4 完成后，facts、projections、context assets 不再混成一个“万能状态层”

---

## Phase 7: User Story 5 - 平台能力扩展能够通过 Capability Plane 接入 (Priority: P2)

**Goal**: 让 provider、tool、future skill/MCP/router 都通过 Capability Plane 接入，而不是重新耦合回 runtime durable state

**Independent Test**: 完成本故事后，团队应能新增一个最小 capability provider 或 tool 能力，而不需要修改 Surface adapter 或 durable facts schema

### Tests for User Story 5

- [ ] T037 [P] [US5] 在 `internal/provider/{fake_test.go,openai_stream_test.go,openai_timeout_test.go}` 与新增 `internal/tools/registry_test.go` 中补 capability invocation、timeout 和 error surface 测试
- [ ] T038 [P] [US5] 在 `tests/architecture/four_plane_boundaries_test.go` 与 `tests/architecture/boundaries_test.go` 中补“Capability Plane 不得推进 durable state、不得拥有 transport 行为”的守护测试

### Implementation for User Story 5

- [ ] T039 [US5] 重构 `internal/provider/types.go`、`internal/tools/{types.go,registry.go}`、`internal/runtime/kernel/contracts.go`、`internal/runner/runner.go`，固定 capability invocation contract 与注册点
- [ ] T040 [P] [US5] 重构 `internal/provider/{openai.go,openai_stream.go,logging.go}` 与 `internal/runner/tool_executor.go`，把 timeout、error typing、trace/log 语义固定在 Runtime <-> Capability 边界
- [ ] T041 [P] [US5] 清理 `internal/tools/{context_get.go,memory_get.go,workspace_file.go,web_fetch.go,web_search.go}` 中潜在的 runtime/store 耦合，确保 capability 只返回结果或错误，不推进 durable state
- [ ] T042 [US5] 运行并记录本故事最小验证：`go test ./internal/provider/... ./internal/tools/... ./internal/runner/... -v`、`make test-unit`

**Checkpoint**: User Story 5 完成后，新增 provider/tool/skill/MCP/router 已有统一 capability 接入方式

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: 收口文档、导航、残余混合 glue 与最终回归，确保四块骨架在仓库里稳定可持续

- [ ] T043 [P] 更新治理与导航文档：`AGENTS.md`、`ARCHITECTURE.md`、`docs/design-docs/{index.md,runtime-flow.md,prompt-and-workspace-context.md}`、`docs/references/testing.md`
- [ ] T044 清理剩余 mixed-owner glue 与过渡 wiring：`internal/bootstrap/app.go`、`internal/runtime/kernel_adapter.go`、`internal/runtime/stream_sink.go`、`internal/http/server.go`
- [ ] T045 [P] 复查并删除多余的 owner 回流点与 stale tests：`tests/architecture/*.go`、`tests/integration/*.go`、`cmd/simiclaw/internal/client/client_test.go`
- [ ] T046 运行 `make fmt`，修复本 feature 涉及文件的格式化问题
- [ ] T047 运行最终验证并把结果同步回 `specs/003-four-plane-architecture-refactor/quickstart.md`：`go test ./tests/architecture/... -v`、`make docs-style`、`make test-unit`、`make test-unit-race-core`、`make accept-current`、`make web-ci`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: 无依赖，可立即开始
- **Phase 2 (Foundational)**: 依赖 Phase 1，并阻塞所有用户故事
- **Phase 3 (US1)**: 依赖 Phase 2；这是整个重构的架构 MVP
- **Phase 4 (US3)**: 依赖 Phase 2；Runtime 内核先收紧 owner，Surface 才有稳定 seam 可消费
- **Phase 5 (US2)**: 依赖 Phase 2，并强烈建议在 US3 的 Runtime seam 稳定后推进
- **Phase 6 (US4)**: 依赖 US1 与 US3；state/context 分离要建立在明确 Runtime owner 之上
- **Phase 7 (US5)**: 依赖 US3；Capability Plane 需要基于已拆开的 Runtime invocation boundary 收口
- **Phase 8 (Polish)**: 依赖所有目标故事完成

### User Story Dependencies

- **US1 (P1)**: 建立统一骨架和 guardrails，是所有后续重构的认知基础
- **US3 (P1)**: 建立 Runtime owner，是 US2/US4/US5 的实现基础
- **US2 (P1)**: 依赖 US3 提供稳定 seams，但完成后能独立验证 Surface adapter 可替换性
- **US4 (P2)**: 依赖 US1/US3 的 owner map 和 runtime contracts
- **US5 (P2)**: 依赖 US3 的 runtime-capability 边界，不依赖 US2 的具体 adapter 形状

### Within Each User Story

- 先补测试和 guardrails，再改实现
- 先固定 seam / model / contract，再改具体 service 或 adapter
- 先完成新 owner 接管，再删除旧路径或过渡 glue
- 每个故事完成后都要跑该故事的最小验证，不把问题拖到最后一起爆

### Parallel Opportunities

- `T002` 与 `T003` 可并行
- `T005`、`T006`、`T007` 可在 `T004` 启动后并行推进
- US1 中 `T009` 与 `T010` 可并行
- US3 中 `T018` 与 `T019` 可并行
- US2 中 `T022`、`T023`、`T024`、`T025` 可按文件范围并行
- US4 中 `T030` 与 `T031` 可并行，`T033`、`T034`、`T035` 可在 `T032` 固定 facts/projections contract 后分支推进
- US5 中 `T037` 与 `T038` 可并行，`T040` 与 `T041` 可并行
- Polish 中 `T043` 与 `T045` 可并行

---

## Parallel Example: User Story 2

```bash
# Launch Surface contract tests together:
Task: "Update internal/http/{ingest/handler_test.go,query/handler_test.go,stream/handler_test.go} and tests/integration/http_ingress_integration_test.go"
Task: "Update cmd/simiclaw/internal/client/client_test.go and web/src/app/router/router.test.tsx"
Task: "Update internal/channels/telegram/{normalize_test.go,runtime_test.go,filter_test.go} and tests/integration/telegram_integration_test.go"

# Launch Surface implementations on disjoint files:
Task: "Refactor internal/http/{ingest/handler.go,query/handler.go,stream/handler.go,server.go}"
Task: "Refactor cmd/simiclaw/internal/{client/client.go,chat/async_cmds.go,inspect/command.go}"
Task: "Refactor internal/channels/telegram/{normalize.go,runtime.go,filter.go}"
Task: "Refactor web/src/lib/api-client.ts"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. 完成 Phase 1 和 Phase 2
2. 完成 Phase 3（US1）
3. 运行 `go test ./tests/architecture/... -v` 和 `make docs-style`
4. 确认 owner map、文档和 guardrails 已稳定
5. 再进入 Runtime 重构

### Incremental Delivery

1. 先交付四块统一骨架和 guardrails（US1）
2. 再交付 Runtime 内核 owner 收紧（US3）
3. 再交付 facts/projections/context separation（US4）
4. 再交付 Surface adapter convergence（US2）
5. 最后交付 Capability Plane 收口与全局 cleanup（US5 + Polish）

### Parallel Team Strategy

1. 团队先一起完成 Setup + Foundational
2. 然后：
   - 开发者 A：US1（文档与架构守护）
   - 开发者 B：US3（Runtime owner 收紧）
3. US3 稳定后：
   - 开发者 C：US2（Surface adapters）
   - 开发者 D：US4（Context/State）
   - 开发者 E：US5（Capability Plane）

---

## Notes

- `[P]` 任务表示文件范围可并行，但仍需遵守阶段依赖
- 不允许为了重构引入兼容层、双路径、旁路写库或旁路发送
- `web/` 始终作为 contract consumer 处理，不纳入后端 owner plane
- 触碰 `/v1/events:ingest`、`/v1/chat:stream`、CLI fallback、web stream consumption 的改动，必须跑 Phase 2/US2 中列出的契约敏感验证
