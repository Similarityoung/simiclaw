# CI/CD 与测试文档 v0.4

> 目标：为当前 `OpenClaw 风格 + Go 单写者 + 无数据库` 架构提供可执行、可分阶段收敛的质量门禁。  
> 对齐 [architecture.md](./architecture.md)、[interfaces.md](./interfaces.md)、[roadmap.md](./roadmap.md)。

## 1. 范围与原则

### 1.1 当前范围

- Runtime 主体：单机单进程，`Gateway + EventLoop + StoreLoop`。
- 持久化：`runtime/sessions.json + runtime/sessions/<session_id>.jsonl + runtime/runs/ + runtime/idempotency/*.jsonl + runtime/cron/jobs.json`。
- 渠道：CLI + Telegram（long polling）。

### 1.2 质量原则

- 正确性优先：同一时刻只允许一条状态推进主链路写盘。
- 可恢复优先：崩溃后可通过 jsonl 事实源恢复，未提交尾部可丢弃。
- 幂等优先：inbound/outbound/action 三层幂等必须可验证。
- 协议优先：HTTP 行为以 `interfaces.md v0.4` 为唯一真相。
- 测试可重复优先：默认使用 mock 与可控时钟，避免网络/时间抖动。

## 2. 仓库与分支规范

### 2.1 分支策略

- `main`：始终可运行、可演示。
- `feat/*`：功能分支。
- 合并策略：PR + CI 全绿 + squash merge。

### 2.2 目录建议

- `cmd/simiclaw`：程序入口。
- `pkg/...`：核心实现与可复用组件。
- `tests/integration`：集成测试。
- `tests/e2e`：端到端测试。
- `.github/workflows/`：CI/CD 工作流。

## 3. 统一测试入口（Make Targets）

M0 起必须提供统一入口：

- `make fmt`
- `make vet`
- `make lint`
- `make test-unit`
- `make test-unit-race-core`
- `make test-integration`
- `make test-e2e-smoke`
- `make test-e2e`
- `make test-fault-injection`
- `make accept-current`
- `make accept-m1`
- `make accept-m2`
- `make accept-m3`
- `make accept-m4`
- `make docs-consistency`

约束：

- CI 只调用上述入口，不直接散落 shell 命令。
- `accept-mX` 允许复用 unit/integration/e2e-smoke，但必须是单一入口。
- 仓库根目录必须提供 `VERSION_STAGE`（例如 `M1`/`M2`/`M3`/`M4`），由 `make accept-current` 与 `make test-e2e-smoke` 读取。
- `make test-integration` 必须固定执行 `go test ./tests/integration/... -tags=integration`，禁止使用 `./...`。
- `make test-unit-race-core` 的 `core` 范围固定为：
  - `./pkg/gateway/...`
- `./pkg/eventing/...`
- `./pkg/persistence/...`
  - `./pkg/idempotency/...`
  - `./pkg/sessionkey/...`
  - 若目录尚未落地，target 可临时 no-op，但必须在输出中明确提示“core package pending”。

## 4. 测试分层（Testing Pyramid）

1. Unit：纯函数/模块行为（快，覆盖广）。
2. Integration：文件持久化、一致性边界、状态机、恢复逻辑。
3. E2E Smoke：最小业务链路（PR 必跑，目标 < 3 分钟）。
4. E2E Full：全量场景（main/nightly 必跑，目标 < 10 分钟）。

建议耗时预算：

- Unit：< 2 分钟
- Integration：< 5 分钟
- E2E Smoke：< 3 分钟
- E2E Full：< 10 分钟

## 5. Unit 测试基线（P0）

### 5.1 协议与建模

- `session_key` canonical 规则：
  - `participant_id_or_dash` 规则固定
  - `thread_id` 不参与 session_key
  - 输出 `sk:<sha256-hex>`
- `payload_hash` canonical 规则：
  - 字段保留/忽略集与接口文档一致
  - 顺序稳定、hash 稳定
- `idempotency_key` 格式校验：
  - `telegram:update:<update_id>`
  - `cli:<conversation_id>:<seq>`

### 5.2 一致性与状态机

- Event 状态机：`accepted -> running -> committed|failed`。
- Delivery 状态机与组合约束：
  - `delivery_status=not_applicable` -> `delivery_detail=not_applicable` 且 `outbox_id` 为空
  - `delivery_detail=direct` -> `outbox_id` 为空
  - `delivery_detail=spooled` -> `outbox_id` 必填
- Approval 幂等转移：`pending -> approved|rejected|expired`。

### 5.3 安全与边界

- `native_ref` 路径校验（仅 `runtime/native/**` 相对路径）。
- `memory_get` 路径白名单与越界拒绝。
- `received_at` 服务端只读：请求侧携带时忽略。

### 5.4 覆盖率门槛（P0）

- `make test-unit` 必须输出覆盖率摘要（`go test -coverprofile` 或等价方式）。
- P0 核心模块最低行覆盖率目标：`>= 80%`。
- P0 核心模块定义：`session_key` 计算、inbound/outbound/action 幂等判定、commit marker 校验、恢复丢尾逻辑。
- PR 门禁可先以“只检查 P0 核心模块覆盖率”落地，仓库整体覆盖率可在 M5 再加硬约束。

## 6. Integration 测试基线（P0/P1）

### 6.1 运行范围与 build tag 隔离

- Integration 测试仅放在 `tests/integration/...`。
- 集成测试文件必须使用 `//go:build integration`。
- 执行命令固定为：`go test ./tests/integration/... -tags=integration`。
- Unit 执行 `go test ./...` 时不得触发 integration。

### 6.2 Ingest 与幂等

- `POST /v1/events:ingest` 首次返回 `202`，重复返回 duplicate。
- `202` 语义保证：返回前已持久化 `event_id/idempotency_key/payload_hash/received_at`。
- `409 CONFLICT` 场景校验 `expected_hash/got_hash`。

### 6.3 StoreLoop 提交边界

固定验证顺序：

1. AppendBatch 写 `<session_id>.jsonl`
2. 写 `runs/<run_id>.json`
3. 更新 `sessions.json`

失败注入测试：

- `runs/<run_id>.json` 写失败时，`sessions.json` 指针不得前移。
- 未收到 StoreLoop ack 前，Outbound 不得发送。

### 6.4 JSONL commit marker 与恢复

- 每个 run batch 尾部必须有 `type=commit`。
- `entry_count` 不含 commit 自身。
- 重启恢复时：
  - 尾部缺 marker -> 丢弃未提交尾部
  - marker 后有脏 entry -> 丢弃尾部
  - `run_id`/`last_entry_id` 校验失败 -> 回滚到前一合法 marker

### 6.5 sessions.json 降级恢复

- `sessions.json` 不可读/为空时，扫描 `runtime/sessions/*.jsonl` 重建最小索引。
- 优先使用 header 的 `session_key`；缺失则挂 `unknown:<session_id>`。

### 6.6 出站与重试

- `outbound_idempotency_key` 重试不重复发送。
- spool 模式下同 key 仅允许一个活跃 outbox 记录。
- adapter 失败后重试状态迁移可观测。

### 6.7 Cron 与 Runtime 字段

- `GET/PUT /v1/cron/jobs` 版本冲突正确返回 `409`。
- PUT 覆盖配置字段且保留 runtime 字段（`last_fired_at/next_fire_at/last_error`）合并语义正确。

### 6.8 Telegram 归一化

- `message`、`callback_query`、附件元信息归一化正确。
- 同一 `update_id` 回放命中幂等去重。
- allowlist 外 `chat_id/user_id` 返回 `403` 且无副作用。

### 6.9 文件隔离与清理策略（P0）

- 每个 integration 用例必须使用独立临时目录（例如 `t.TempDir()`）作为 workspace。
- 测试必须在临时目录内生成 `sessions.json/jsonl/runs/idempotency` 文件，禁止写入仓库固定路径。
- 默认允许 `go test -parallel` 并发执行；若用例存在共享资源，不得标记 `t.Parallel()`，必须串行执行并注明原因。
- 用例结束后依赖测试框架自动清理临时目录，不得残留 runtime 文件到仓库工作树。

## 7. E2E 场景（与 roadmap 对齐）

### 7.1 E2E Smoke（PR 必跑）

- 按 `VERSION_STAGE` 增量执行（必须可在 M1 起通过）：
  - M1：ingest -> commit -> outbound 基础链路 + NO_REPLY 抑制基础断言
  - M2：在 M1 基础上增加查询/恢复 smoke
  - M3：在 M2 基础上增加记忆写入与召回
  - M4：在 M3 基础上增加审批闭环与 patch/rollback smoke

### 7.2 E2E Full（main/nightly 必跑）

- NO_REPLY 内务回合：`memory_flush/compaction` 无用户可见 outbound。
- 崩溃恢复：制造尾部半截 batch，重启后可继续处理。
- 幂等反查：`GET /v1/events:lookup?idempotency_key=...` 返回 `event_id`。
- Telegram 归一化全量回归。

### 7.3 执行约束

- Telegram 相关 E2E 禁止连接真实 Telegram 网络。
- Telegram E2E 统一使用本地模拟 update（或直接 ingest 归一化事件）。
- 真实 Telegram 仅用于人工演示，不进入 CI。

## 8. Mock、时钟与可重复性

### 8.1 Mock 产物（M0/M1 硬交付）

- `MockProvider`：脚本化返回 `assistant(tool_calls) -> tool_result -> assistant(final)`。
- `FakeToolRegistry` 或等价 fake executor。
- Process runner mock：固定 stdin/stdout JSON，stderr 可控。

### 8.2 时间与调度约束

- 任何测试默认禁止直接依赖真实 `time.Now()`/`time.Sleep()`。
- 允许例外：仅限服务启动探活等极少数场景。
- Cron 测试优先使用手动 fire/可控 Clock。

### 8.3 故障注入策略

- PR/main 必跑 deterministic 故障注入：
  - StoreLoop 指定第 N 次写失败
  - Adapter 指定前 N 次发送失败
- nightly 可选扩展随机扰动，但不得替代 deterministic 套件。

## 9. CI 工作流（GitHub Actions）

### 9.1 PR 必跑（快速门禁）

1. `make fmt`
2. `make vet`
3. `make lint`
4. `make test-unit`
5. `make test-unit-race-core`
6. `make test-integration`
7. `make test-e2e-smoke`
8. `make accept-current`

### 9.2 main/nightly 增量门禁

- `make test-e2e`
- `make test-fault-injection`
- nightly 可选附加随机故障扰动

### 9.3 阶段门禁矩阵

- M0：`make fmt + make vet + make lint + make test-unit + make test-unit-race-core`
- M1：`M0 + make test-integration + make accept-m1`
- M2：`M1 + make test-integration（recovery/query regression）+ make accept-m2`
- M3：`M2 + make test-e2e-smoke（memory/no_reply）+ make accept-m3`
- M4：`M3 + make test-e2e-smoke（approval/patch/rollback）+ make accept-m4`
- M5（可选）：`M4 + make test-fault-injection`

### 9.4 文档一致性检查（新增 job）

新增 `docs-consistency` job（建议 < 1 分钟）：

- 检查关键文档版本头是否一致（当前 `v0.4`）。
- 检查禁用旧术语：旧版 lane worker 叙述、旧数据库持久化文件名、旧迁移流程叙述等。
- 检查 roadmap 与 cicd 的里程碑编号是否一致。
- 检查 `interfaces.md` 中 `format_version` 与 `session_key` 规则未出现多套冲突定义。
- 检查 Roadmap 与 CI 文档中的 Make target（尤其 `accept-m*` / `accept-current`）一致。

### 9.5 最小 `ci.yml` 示例

```yaml
name: ci
on:
  pull_request:
  push:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: make fmt
      - run: make vet
      - run: make lint
      - run: make test-unit
      - run: make test-unit-race-core
      - run: make test-integration
      - run: make test-e2e-smoke
      - run: make accept-current
  docs-consistency:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: make docs-consistency
```

## 10. 无发版模式约定

- 当前阶段不要求 `.github/workflows/release.yml`、tag 发布、SHA256 与发布产物上传。
- 质量保障以 PR/main 门禁与 `accept-current` 为主，M1-M4 通过即视为可继续推进。
- 若后续需要对外分发，再新增 release 流程与对应门禁。

## 11. 质量门禁（DoD）

- P0 测试全绿：幂等、一致性、恢复、安全。
- `runs` 与 `events` 可追溯：`event_id -> run_id -> commit_id/outbox_id`。
- 任意崩溃点重启后不出现不可恢复脏状态。
- 无数据库依赖残留（代码、测试、文档均不含数据库持久化/迁移前置条件）。
- 无发版模式下不要求 release 工件；只要求开发期门禁持续全绿。

## 12. 与 roadmap 对齐映射

- M0 对应：工程骨架、统一 Make 入口、Mock 产物。
- M1 对应：ingest 主链路与提交边界测试。
- M2 对应：查询接口与崩溃恢复测试。
- M3 对应：memory/RAG/NO_REPLY 测试。
- M4 对应：approval/patch/rollback 测试。
- M5 对应：故障注入常态化（可选，不阻塞主线）。
