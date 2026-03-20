# Tasks: 日志系统升级与链路观测补齐

**Input**: Design documents from `/specs/002-logging-upgrade/`
**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `analysis.md`, `quickstart.md`

**Tests**: 本特性在规格里明确要求 logger 输出测试、代表性包级测试、架构测试、单元测试与 acceptance 回归，因此任务中包含对应验证项。

**Organization**: 任务按共享基础与用户故事分组，确保每个故事都能独立实现、独立验证，并可作为阶段性交付单元推进。

## Format: `[ID] [P?] [Story] Description`

- **[P]**: 可以并行执行，前提是修改文件不冲突
- **[Story]**: 对应的用户故事，例如 `US1`
- 每条任务都包含明确文件路径，避免“大而空”的实现项

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 先把 canonical log line 的测试基线立住，避免后续各链路补点时输出形状漂移

- [ ] T001 [P] 在 `pkg/logging/logger_test.go` 中重写 console 输出断言，固定目标单行格式、`[module] message` 风格和字段区 `key=value` 形状
- [ ] T002 [P] 在 `pkg/logging/logger_internal_test.go` 中补特殊字符、错误字段、空字段和 `Sync` 行为的内部断言，作为全 feature 的共享 guardrail

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 完成所有用户故事共享的 logger 核心升级与字段约定；本阶段完成前不要开始主链路补点

**⚠️ CRITICAL**: 本阶段完成前，不应进入任何用户故事实现

- [ ] T003 在 `pkg/logging/logger.go` 中把 console encoder 输出改为人类可读的单行 `key=value` 字段格式，同时保留 timestamp、level、caller 和 `[module] message`
- [ ] T004 在 `pkg/logging/logger.go` 中收敛字段转义、稳定顺序、错误字段与空值输出策略，不改变 `Init`、`ParseLevel`、`L(module)`、`With`、`Sync` 的现有 API
- [ ] T005 在 `pkg/logging/logger.go` 与 `pkg/logging/logger_test.go` 中定义并固定 canonical correlation field contract，统一 `event_id`、`run_id`、`session_key`、`payload_type`、`outbox_id`、`job_id`、`worker`、`tool_call_id`、`tool_name`、`provider`、`model` 等共享字段命名与展示顺序
- [ ] T006 运行 `go test ./pkg/logging/...`，修正 logger 核心回归，作为所有故事的阻塞前置

**Checkpoint**: canonical line shape 已固定，后续链路补点可以在统一输出契约上推进

---

## Phase 3: User Story 1 - 运维可从单行日志追踪 event 全链路 (Priority: P1) 🎯 MVP

**Goal**: 让一次 event 从 ingress 到 finalize/outbound 的关键阶段在 `info` 级别下可读、可串联、可定位

**Independent Test**: 完成本故事后，团队应能通过一次 `POST /v1/events:ingest` 或 `POST /v1/chat:stream` 请求，在日志中看到 `ingest -> persist/enqueue -> runtime start/finalize -> outbound` 主链路，并通过 `event_id/run_id/session_key` 串起来

### Tests for User Story 1

- [ ] T007 [P] [US1] 在 `internal/http/middleware/api_key_test.go` 和新增 `internal/http/ingest/handler_test.go` 中补 API key 拒绝、JSON decode 失败和写入口错误日志断言
- [ ] T008 [P] [US1] 在 `internal/gateway/service_test.go`、`internal/runtime/kernel/kernel_test.go`、`internal/outbound/delivery/worker_test.go`、`cmd/simiclaw/internal/gateway/command_test.go` 与新增 `internal/bootstrap/app_test.go` 中补 persist/enqueue、claim/finalize、send/retry/dead-letter、启动失败路径的代表性日志断言

### Implementation for User Story 1

- [ ] T009 [US1] 在 `cmd/simiclaw/internal/gateway/command.go` 补服务启动、启动入口失败透传和优雅停止边界日志
- [ ] T010 [US1] 在 `internal/bootstrap/app.go` 补 DB open、provider factory、Telegram runtime 创建、supervisor start 与 `App.Start` 失败日志，覆盖 major subsystem startup failure owner
- [ ] T011 [US1] 在 `internal/http/middleware/api_key.go`、`internal/http/ingest/handler.go`、`internal/http/stream/handler.go` 补 ingress 鉴权/解码/streaming unsupported 日志，并避免 keepalive/空轮询进入 `info`
- [ ] T012 [US1] 在 `internal/gateway/service.go` 补 validate、binding、routing、rate limit、persist、duplicate、enqueue 里程碑日志，明确区分“已接受但未入队”和“已开始执行”
- [ ] T013 [US1] 在 `internal/runtime/eventloop.go` 与 `internal/runtime/kernel/service.go` 补 enqueue、repump 摘要、claim、execute start、finalize start/complete/fail 里程碑日志，保持两阶段语义不变
- [ ] T014 [US1] 在 `internal/outbound/delivery/worker.go` 统一 send failed、retry scheduled、dead-letter、sent 的字段集合、日志级别和关联 ID
- [ ] T015 [US1] 运行 `go test ./internal/http/... ./internal/gateway/... ./internal/runtime/... ./internal/outbound/...` 与 `go test ./tests/architecture/... -v`，确认主链路日志补点没有破坏边界和行为

**Checkpoint**: User Story 1 完成后，正常 event 主链路已经具备可读文本日志和稳定关联字段，可作为本 feature 的 MVP 演示

---

## Phase 4: User Story 2 - 开发者可快速定位 runner/provider/tool 链路问题 (Priority: P1)

**Goal**: 让 provider 调用、tool 执行和 terminal outcome 在 runner owner 下具备最小可诊断日志，同时默认做摘要/脱敏

**Independent Test**: 完成本故事后，团队应能在 provider 超时、tool deny/fail、`max_tool_rounds` 命中等场景下，仅通过 runner/provider/tool 日志定位失败阶段、模型、工具和耗时

### Tests for User Story 2

- [ ] T016 [P] [US2] 在 `internal/runner/runner_test.go` 中补 payload plan、provider start/end/failure、tool rounds、tool deny/fail、terminal outcome 的代表性日志断言
- [ ] T017 [P] [US2] 在 `internal/provider/openai_timeout_test.go` 与 `internal/provider/openai_stream_test.go` 中补 timeout/stream failure error surface 和“不泄漏 prompt 正文”的断言

### Implementation for User Story 2

- [ ] T018 [US2] 在 `internal/runner/runner.go` 增加 payload plan、run mode、provider start/end/timeout/fail、finish reason、token usage、latency 和 terminal outcome 日志，并让 runner 成为 provider 终态日志 owner
- [ ] T019 [P] [US2] 在 `internal/runner/tool_executor.go` 与 `internal/runner/display.go` 收敛 tool start/finish/deny/fail 的摘要日志、截断规则和敏感字段脱敏
- [ ] T020 [US2] 在 `internal/provider/openai.go` 与 `internal/provider/openai_stream.go` 只保留必要的底层 transport / `debug` 诊断，确保不重复打印 terminal `ERROR`，也不打印 prompt 原文
- [ ] T021 [US2] 运行 `go test ./internal/runner/... ./internal/provider/...` 与 `make test-unit`，确认事务外执行链路日志与现有行为保持一致

**Checkpoint**: User Story 2 完成后，runner/provider/tool 链路已能独立排障，不必依赖 trace 查询或断点调试才能定位主要失败点

---

## Phase 5: User Story 3 - 后台 worker 与恢复路径具备足够可观测性 (Priority: P2)

**Goal**: 让 processing recovery、scheduled jobs、Telegram polling 等后台路径在成功、失败、空转和恢复时具备一致、低噪声的日志

**Independent Test**: 完成本故事后，团队应能通过 recovery、retry、cron ingest、Telegram poller 等场景，从日志判断 worker 是否在运行、做了什么、结果如何，而不会被 idle 噪声淹没

### Tests for User Story 3

- [ ] T022 [P] [US3] 在 `internal/runtime/workers_test.go` 中补 processing recovery、scheduled jobs 与 delivery worker 的数量、决策和失败摘要日志断言
- [ ] T023 [P] [US3] 在 `internal/channels/telegram/runtime_test.go` 中补 Telegram handler/poller/heartbeat/ignored update 的字段和级别一致性断言

### Implementation for User Story 3

- [ ] T024 [US3] 在 `internal/runtime/workers/processing_recovery.go` 补 recovery 数量、requeue 结果和失败摘要日志，并把 idle 空转控制在 `debug`
- [ ] T025 [P] [US3] 在 `internal/runtime/workers/scheduled_jobs.go` 补 claim、ingest、duplicate、enqueue、fail/retry job 日志，包含 `job_id`、job kind、`event_id`
- [ ] T026 [P] [US3] 在 `internal/runtime/supervisor.go` 与 `internal/channels/telegram/runtime.go` 补 worker lifecycle、heartbeat 和 channel owner 日志，并统一字段名与级别语义
- [ ] T027 [US3] 运行 `go test ./internal/runtime/... ./internal/channels/telegram/...` 与 `make test-unit`，确认后台 worker 与 channel 路径的日志补点稳定

**Checkpoint**: User Story 3 完成后，后台 worker 和 Telegram runtime 已具备一致的可观测性，维护者可以区分 idle、recovery、retry 和故障

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: 收口文档、噪声控制与最终回归，确保 feature 的验证路径和仓库参考文档同步

- [ ] T028 [P] 更新 `docs/references/configuration.md`、`docs/references/testing.md` 与 `specs/002-logging-upgrade/quickstart.md`，说明默认人类可读日志、验证命令和 `api_key` 场景注意事项
- [ ] T029 [P] 复查 `internal/http/stream/handler.go`、`internal/runtime/workers/*.go`、`internal/provider/*.go` 中的 keepalive、空转和 transport 噪声，把非里程碑日志压回 `debug`
- [ ] T030 运行 `make fmt`，修复本特性涉及文件的格式化问题
- [ ] T031 运行 `go test ./tests/architecture/... -v`、`make test-unit`、`make accept-current`，并把最终验证结果同步回 `specs/002-logging-upgrade/quickstart.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: 无依赖，可立即开始
- **Phase 2 (Foundational)**: 依赖 Phase 1，并阻塞所有用户故事
- **Phase 3 (US1)**: 依赖 Phase 2；这是本 feature 的 MVP
- **Phase 4 (US2)**: 依赖 Phase 2；可与 US1 并行，但应复用已固定的 canonical log line 与字段命名
- **Phase 5 (US3)**: 依赖 Phase 2；单人推进时建议在 US1 的主链路字段稳定后再展开
- **Phase 6 (Polish)**: 依赖所有目标用户故事完成

### User Story Dependencies

- **US1 (P1)**: 只依赖 logger 核心升级，是最先可交付的主链路观测增量
- **US2 (P1)**: 只依赖 logger 核心升级；应避免把 provider 终态日志责任下沉到 provider 层
- **US3 (P2)**: 只依赖 logger 核心升级；应沿用 US1/US2 已确定的字段和级别语义

### Within Each User Story

- 先补代表性测试和断言，再做实现补点
- 先补 owner 层日志，再决定是否真的需要下探到底层补 `debug`
- 先保证摘要/脱敏和级别分层，再增加新的日志点
- 每个故事完成后都要跑对应最小验证，不把问题堆到最后一起排

### Parallel Opportunities

- `T001` 与 `T002` 可并行
- `T007` 与 `T008` 可并行
- `T016` 与 `T017` 可并行
- `T019` 与 `T020` 可并行
- `T022` 与 `T023` 可并行
- `T025` 与 `T026` 可并行
- `T028` 与 `T029` 可并行

---

## Implementation Strategy

### MVP First (US1 Only)

1. 完成 Phase 1 和 Phase 2
2. 完成 Phase 3（US1）
3. 运行 `go test ./internal/http/... ./internal/gateway/... ./internal/runtime/... ./internal/outbound/...`
4. 运行 `go test ./tests/architecture/... -v`
5. 如需要，再运行 `make test-unit`

### Incremental Delivery

1. 先交付 canonical log line
2. 再交付 event 主链路日志（US1）
3. 再交付 runner/provider/tool 诊断能力（US2）
4. 最后交付 worker / Telegram / docs / final regression（US3 + Polish）

### Notes

- `[P]` 任务表示文件范围可并行，但仍需遵守阶段依赖
- 不为日志补点引入新的日志框架、兼容层或全局 observability 抽象
- 不为补字段而跨层查库或破坏现有模块边界
- 所有终态错误都应遵守“最有上下文的一层记录一次”的原则
