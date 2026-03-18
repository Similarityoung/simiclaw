# Tasks: Runtime Kernel Refactor

**Input**: Design documents from `/specs/001-runtime-kernel-refactor/`
**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/kernel-boundaries.md`

**Tests**: 本特性明确要求分 slice 验证，因此任务中包含架构测试、单元测试、并发/race 测试与必要的集成验证。

**Organization**: 任务按共享基础与用户故事分组，保证每个故事都能独立实现、独立验证，并能作为阶段性交付单元。

## Format: `[ID] [P?] [Story] Description`

- **[P]**: 可以并行执行，前提是修改文件不冲突
- **[Story]**: 对应的用户故事，例如 `US1`
- 每条任务都包含明确文件路径，避免“大而空”的重构项

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 建立目标目录骨架和第一层守护测试，确保后续重构有清晰落点

- [X] T001 创建目标目录与包注释骨架：`internal/runtime/{kernel,payload,workers,lanes,model}`、`internal/gateway/{bindings,routing,model}`、`internal/http/{ingest,query,stream,middleware}`、`internal/store/{tx,projections,queries}`、`internal/runner/{context,tools,model}`
- [X] T002 [P] 在 `tests/architecture/runtime_kernel_boundaries_test.go` 与 `tests/architecture/http_channel_boundaries_test.go` 中补充新包边界约束，禁止 `runtime/gateway/http/channels/runner/query` 直接依赖 `internal/store`
- [X] T003 [P] 在 `docs/design-docs/` 下创建迁移设计占位文档，例如 `docs/design-docs/runtime-kernel-refactor.md`，记录新目录 owner 与迁移顺序

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 定义所有故事共享的核心契约与装配方式；完成前不要开始故事实现

**⚠️ CRITICAL**: 本阶段完成前，不应进入任何用户故事实现

- [X] T004 定义 runtime 核心契约与 DTO：更新 `internal/runtime/model/types.go`，新增 `internal/runtime/kernel/contracts.go`
- [X] T005 [P] 定义 gateway 统一入口 DTO 与绑定/路由契约：`internal/gateway/model/types.go`、`internal/gateway/bindings/contracts.go`、`internal/gateway/routing/contracts.go`
- [X] T006 [P] 定义事实层边界与 runtime event sink 契约：`internal/runtime/kernel/facts.go`、`internal/runtime/kernel/events.go`
- [ ] T007 [P] 创建事实层事务入口文件：`internal/store/tx/{ingest_event.go,claim_work.go,finalize_run.go,outbox.go,scheduled_jobs.go,recover_processing.go}`
- [ ] T008 更新装配入口，使依赖通过契约注入：`internal/bootstrap/app.go`、`cmd/simiclaw/`
- [ ] T009 更新架构与编译守护测试，确保新目录可单独编译：`tests/architecture/`、相关包级 `*_test.go`

**Checkpoint**: 内核、gateway、HTTP/channels、事实层的基础契约已经固定，后续故事可以在此之上推进

---

## Phase 3: User Story 1 - 重建 Runtime Kernel (Priority: P1) 🎯 MVP

**Goal**: 把 `claim -> execute -> finalize` 主链路从中心文件中抽出，落到显式 kernel owner

**Independent Test**: 仅完成本故事时，`runtime` 应能通过新 kernel 处理 event/job/outbox work，同时保持现有 SQLite 语义与 `outbox-after-commit`

### Tests for User Story 1

- [X] T010 [P] [US1] 在 `internal/runtime/kernel/kernel_test.go` 中补充 kernel 生命周期测试，覆盖 claim、execute、finalize、failure 路径
- [X] T011 [P] [US1] 在 `tests/integration/runtime_kernel_integration_test.go` 中补充端到端验证，确认运行不变量与持久化语义未变
- [X] T012 [P] [US1] 在 `internal/runtime/kernel/kernel_race_test.go` 或现有 race 测试文件中增加 worker lifecycle / goroutine stop path 校验

### Implementation for User Story 1

- [X] T013 [US1] 将 `internal/runtime/eventloop.go` 的主执行编排抽到 `internal/runtime/kernel/service.go`
- [X] T014 [P] [US1] 将 claim/finalize 命令组装拆到 `internal/runtime/kernel/{claim.go,finalize.go}`
- [ ] T015 [P] [US1] 创建 payload 分派与内置 handler：`internal/runtime/payload/{handler.go,registry.go,message.go,memory_flush.go,compaction.go,cron_fire.go}`
- [ ] T016 [P] [US1] 将 `internal/runtime/workers.go` 拆为具名 owner：`internal/runtime/workers/{heartbeat.go,processing_recovery.go,scheduled_jobs.go,delivery_poll.go}`
- [ ] T017 [US1] 通过 `internal/store/tx/{claim_work.go,finalize_run.go,...}` 接回事实层，不再让 kernel 直接感知 `store` 行级结构
- [ ] T018 [US1] 更新 `internal/bootstrap/app.go`，让 runtime 通过 kernel contracts 装配，并收敛旧 `eventloop` 入口
- [X] T019 [US1] 运行并记录最小验证：`go test ./tests/architecture/... -v`、`make test-unit`、`make test-unit-race-core`、必要时 `make accept-current`

**Checkpoint**: User Story 1 完成后，runtime 主链路已经有明确 owner，且可以在不依赖 HTTP/channels 重构的前提下独立验证

---

## Phase 4: User Story 2 - 建立显式扩展点与统一外部边界 (Priority: P1)

**Goal**: 让 HTTP 与 channels 通过统一 ingress/egress 契约接入系统，并把 payload/worker 扩展点显式化

**Independent Test**: 仅完成本故事时，维护者应能新增一个最小 payload handler、一个最小 channel integration，而不改核心事务语义

### Tests for User Story 2

- [ ] T020 [P] [US2] 在 `internal/runtime/payload/registry_test.go` 与 `internal/runtime/workers/registry_test.go` 中验证 handler/worker 注册机制
- [ ] T021 [P] [US2] 在 `tests/integration/http_ingress_integration_test.go` 中验证 HTTP ingress 统一进入 gateway
- [ ] T022 [P] [US2] 在 `tests/integration/telegram_integration_test.go` 与 `tests/integration/cli_integration_test.go` 中验证 channel 输入归一化与 gateway 接入

### Implementation for User Story 2

- [ ] T023 [US2] 将 session key/scope/binding 规则从 `internal/session/{key.go,scope.go}` 与 `internal/ingest/resolver.go` 收拢到 `internal/gateway/bindings/{key.go,scope.go,hints.go,resolver.go}`
- [ ] T024 [US2] 构建 gateway 主入口与路由决策：`internal/gateway/service.go`、`internal/gateway/routing/{service.go,decision.go}`
- [ ] T025 [P] [US2] 将 `internal/httpapi/` 迁移为 `internal/http/{ingest,query,stream,middleware}/`，分离写入口、读接口、SSE/watch 与 middleware
- [ ] T026 [P] [US2] 更新 `internal/channels/{cli,telegram}/`，让各边界在自身完成 normalize，并只向 gateway 发送统一 ingress
- [ ] T027 [P] [US2] 显式化 payload 与 worker 注册入口：`internal/runtime/payload/registry.go`、`internal/runtime/workers/registry.go`
- [ ] T028 [US2] 清理外部边界对旧入口的直接依赖，禁止 `http/channels/gateway` 继续穿透 `internal/store`
- [ ] T029 [US2] 下线或删除已被替代的旧入口路径：`internal/session/`、`internal/ingeststore/`、被新 gateway/http 流程替代的旧 `internal/ingest` 入口
- [ ] T030 [US2] 运行并记录最小验证：`go test ./tests/architecture/... -v`、`make test-unit`、与 HTTP/channel 相关的 targeted integration tests

**Checkpoint**: User Story 2 完成后，新增 HTTP/channel/payload/worker 能力已经有明确接入点，不再依赖中心分支文件

---

## Phase 5: User Story 3 - 提升数据流可追踪性与后台工作可观测性 (Priority: P2)

**Goal**: 让维护者可以从代码入口与文档快速定位 ingest、execute、finalize、delivery、worker owner

**Independent Test**: 仅完成本故事时，团队应能沿着文档和代码入口追到一条 event 的完整路径，并区分每个 worker 的职责与 stop path

### Tests for User Story 3

- [ ] T031 [P] [US3] 在 `tests/integration/runtime_trace_path_test.go` 中验证 ingest -> claim -> execute -> finalize -> delivery 的可追踪路径
- [ ] T032 [P] [US3] 在 `internal/runtime/workers/registry_test.go` 或 `internal/runtime/workers/*_test.go` 中验证 worker role metadata、heartbeat name、failure strategy

### Implementation for User Story 3

- [ ] T033 [US3] 在 `internal/runtime/kernel/events.go` 与 `internal/runtime/kernel/service.go` 中定义并发布 runtime events
- [ ] T034 [P] [US3] 将 `internal/http/stream/` 建立在 runtime events 之上，替代旧的隐式 streaming 入口
- [ ] T035 [P] [US3] 将 read model 统一收敛到 `internal/query/model/`，并更新 `internal/http/query/` 的映射与读取路径
- [ ] T036 [US3] 更新 `docs/design-docs/runtime-flow.md`、`docs/design-docs/module-boundaries.md`、`ARCHITECTURE.md`，明确新的 owner 与主链路入口
- [ ] T037 [US3] 运行并记录最小验证：`make test-unit`、`make test-integration`、必要时 `make accept-current`

**Checkpoint**: User Story 3 完成后，维护者不依赖口口相传也能看懂主链路与后台 worker 的归属

---

## Phase 6: User Story 4 - 完成渐进迁移并引入 Lane-Ready 内核 (Priority: P3)

**Goal**: 在不改变事实模型的前提下，完成目录与实现迁移，并给 future concurrency 留出显式入口

**Independent Test**: 仅完成本故事时，项目应具备 lane-ready hooks、新的 store/query 形状，以及可回滚的迁移检查点

### Tests for User Story 4

- [ ] T038 [P] [US4] 在 `internal/runtime/lanes/{key_test.go,scheduler_test.go}` 中补充 lane key 与串行化策略测试
- [ ] T039 [P] [US4] 在 `tests/integration/runtime_lanes_test.go` 中验证 lane hooks 不破坏现有两阶段运行语义

### Implementation for User Story 4

- [ ] T040 [US4] 引入 lane key 与调度 hooks：`internal/runtime/lanes/{key.go,scheduler.go}`
- [ ] T041 [US4] 将 `internal/store/{events.go,runs.go,sessions.go,outbox.go,history.go,list_queries.go,query_models.go}` 按职责迁到 `internal/store/{tx,projections,queries}/`
- [ ] T042 [US4] 将 runner 上下文组装重构到 `internal/runner/context/{history.go,memory.go,assembler.go}`，并明确与 `internal/memory/` 的长期记忆边界
- [ ] T043 [US4] 将 `internal/readmodel/` 彻底并入 `internal/query/`，并删除旧 readmodel 兼容路径
- [ ] T044 [US4] 删除已不再使用的旧实现与兼容 wiring，确保 bootstrap 只装配新目录体系
- [ ] T045 [US4] 更新 `specs/001-runtime-kernel-refactor/quickstart.md` 与迁移设计文档，补上 slice rollback / validation checkpoints
- [ ] T046 [US4] 运行全量验证：`go test ./tests/architecture/... -v`、`make test-unit`、`make test-unit-race-core`、`make accept-current`、targeted integration tests

**Checkpoint**: 所有目标目录与核心职责已经迁到新形状，后续新增 concurrency/delivery/channel 能力无需再做顶层重写

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: 清理迁移遗留、补齐文档与最终验证

- [ ] T047 [P] 更新导航与参考文档：`AGENTS.md`、`docs/index.md`、`docs/references/testing.md`
- [ ] T048 清理死代码、旧 import、旧包注释与冗余 shim，确保没有遗留“新旧双路径”
- [ ] T049 运行 `make fmt` 并修复格式化问题
- [ ] T050 汇总本次迁移的验证结果与遗留事项到 `specs/001-runtime-kernel-refactor/quickstart.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: 无依赖，可立即开始
- **Phase 2 (Foundational)**: 依赖 Phase 1，且会阻塞所有用户故事
- **Phase 3 (US1)**: 依赖 Phase 2；这是后续故事最稳的内核基础
- **Phase 4 (US2)**: 依赖 Phase 2，建议在 US1 核心 contracts 落稳后推进
- **Phase 5 (US3)**: 依赖 US1 与 US2 的基础结构，尤其依赖 runtime events 与新的 HTTP/Query 入口
- **Phase 6 (US4)**: 依赖前面所有阶段；这是收口迁移与 lane-ready 的阶段
- **Phase 7 (Polish)**: 依赖所有目标阶段完成

### User Story Dependencies

- **US1 (P1)**: 建立 runtime kernel 主体，是整个重构的 MVP
- **US2 (P1)**: 建立扩展点和统一边界，依赖 foundational contracts，建议在 US1 基础上开展
- **US3 (P2)**: 依赖 US1 的 kernel owner 和 US2 的边界收敛
- **US4 (P3)**: 依赖 US1-US3，负责迁移收口与并发入口预留

### Within Each User Story

- 先补测试和边界守护，再做实现迁移
- 先定义 contracts/model，再改 service/usecase
- 先完成新路径装配，再删除旧路径
- 每个故事完成后都要跑对应最小验证，不把问题拖到最后一起爆

### Parallel Opportunities

- `T002` 与 `T003` 可并行
- `T004` 之后，`T005`、`T006`、`T007` 可并行
- US1 中 `T014`、`T015`、`T016` 可并行
- US2 中 `T025`、`T026`、`T027` 可并行
- US3 中 `T034` 与 `T035` 可并行
- US4 中 `T038` 与 `T039` 可并行，`T041` 与 `T042` 可分支推进后再汇合

---

## Implementation Strategy

### MVP First (US1 Only)

1. 完成 Phase 1 和 Phase 2
2. 完成 Phase 3（US1）
3. 运行 `go test ./tests/architecture/... -v`、`make test-unit`、`make test-unit-race-core`
4. 必要时运行 `make accept-current`
5. 在 US1 稳定后再进入 US2

### Incremental Delivery

1. Setup + Foundational 完成后，先交付 runtime kernel
2. 再交付 gateway/http/channels 的统一边界
3. 再交付 traceability 与 worker owner clarity
4. 最后完成 lanes 与目录迁移收口

### Notes

- `[P]` 任务表示文件修改范围可并行，但仍需尊重阶段依赖
- 任何涉及 `internal/store` 的改动都必须保事实语义不变
- 删除旧目录前，必须确认 bootstrap 已只依赖新路径
- 若某个 slice 无法独立验证，则该 slice 还不算完成
