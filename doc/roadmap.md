# Roadmap v0.4（里程碑 + 交付物 + 验收）

> 本文基于 [requirements.md](./requirements.md)、[architecture.md](./architecture.md)、[interfaces.md](./interfaces.md)、[cicd-testing.md](./cicd-testing.md)。  
> 目标：按“可实现、可运维、可演示”推进 OpenClaw 风格单机 Runtime（Go 单写者、无数据库）。

## 1. 版本定位（v0.4）

- 单机单进程，正确性优先于吞吐。
- 持久化以文件为中心：`runtime/sessions.json + runtime/sessions/<session_id>.jsonl + runtime/runs/ + runtime/idempotency/*.jsonl + runtime/cron/jobs.json`。
- 一致性由 `EventLoop + StoreLoop + AppendBatch(commit marker)` 保证，不依赖数据库事务。
- 先做 CLI + Telegram 两入口；其余渠道后置。

## 2. 里程碑总览

- M0（基础骨架）：服务可启动，workspace 可初始化，StoreLoop 原子写可用。
- M1（事件主链路）：`ingest -> EventLoop -> AgentRun -> StoreLoop commit -> outbound` 跑通。
- M2（可观测与可恢复）：事件状态查询、run/sessions 查询、崩溃恢复可验证。
- M3（记忆/RAG/静默回合）：`memory_search/memory_get`、NO_REPLY、compaction 闭环。
- M4（审批与演化）：approval 闭环、patch apply/rollback、最小回归护栏。
- M5（可选硬化，可跳过）：故障注入与演示脚本稳定复现。

### 2.1 计划工期（估算）

以下时间以 `2026-03-03` 为计划起点，按单团队并行度有限场景估算：

- M0：1 周，目标完成 `2026-03-10`
- M1：2 周，目标完成 `2026-03-24`
- M2：1 周，目标完成 `2026-03-31`
- M3：2 周，目标完成 `2026-04-14`
- M4：2 周，目标完成 `2026-04-28`
- M5（可选）：1-2 周，目标完成 `2026-05-12`

说明：该时间表用于风险跟踪，不作为对外承诺；若 scope 变化应同步更新本文。

## 3. Phase 0：工程骨架（M0）

### 3.1 目标

建立可运行最小系统：配置、目录、日志、单写者写入框架。

### 3.2 交付物

- `cmd/simiclaw`：`serve/init` 子命令。
- 统一测试入口（Make 或等价脚本）：
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
- M0 强制可用目标：`fmt/vet/lint/test-unit/test-unit-race-core/accept-current/docs-consistency`。
- 其余目标在 M0 允许先提供占位实现（no-op 或输出“stage not ready”），但命令名必须存在且退出码可预测。
- `VERSION_STAGE` 文件（机器可读，示例值：`M1`/`M2`/`M3`/`M4`），用于驱动 `accept-current` 与分阶段 smoke 用例选择。
- workspace 初始化：
  - `runtime/sessions.json`
  - `runtime/sessions/`
  - `runtime/runs/`
  - `runtime/idempotency/`
  - `runtime/cron/jobs.json`
- `StoreLoop` 原子写工具（`tmp + fsync + rename`）。
- 结构化日志字段：`event_id/run_id/session_key/session_id/commit_id`。
- 可重复测试基础设施：
  - M0 硬交付：可控 `Clock` + `MockProvider` 最小骨架。
  - M1 补齐：`FakeToolRegistry`（或等价 fake executor）与脚本化回放能力。

### 3.3 验收

- `simiclaw serve --config ...` 可启动。
- `simiclaw init --workspace ...` 生成目录结构。
- 人工 kill 进程后重启，`sessions.json` 不损坏。

## 4. Phase 1：事件主链路（M1）

### 4.1 目标

跑通接口文档定义的核心链路与幂等语义。

### 4.2 交付物

- `POST /v1/events:ingest`
  - 入站幂等（`idempotency_key` + canonical `payload_hash`）
  - `ResolveSession(session_key -> active_session_id)`
  - `202` 前持久化最小事件元信息（`event_id/idempotency_key/payload_hash/received_at`）
- EventLoop（串行推进）与 Runner 调用骨架。
- StoreLoop commit 顺序：
  1. `AppendBatch` 到 `<session_id>.jsonl`
  2. 写 `runs/<run_id>.json`
  3. 更新 `sessions.json`
- Outbound 发送前置：必须等待 StoreLoop ack。
- `GET /v1/events/{event_id}` 基础状态查询。
- 健康检查端点：`GET /healthz`（存活）、`GET /readyz`（就绪）。
- 里程碑验收入口：`make accept-current`（在 M1 阶段等价于 `make accept-m1`，供 PR/main workflow 调用）。

M1 偏差说明（已记录）：

- Telegram 在 M1 先实现“协议归一化 + fixture 回放测试”，不接真实 long polling 网络链路。
- 回补计划：M2 增补 Telegram long polling 生产接入，并复用现有归一化函数与幂等测试夹具，避免协议分叉。
- DLQ Retry Worker 骨架：`dead_letters/events.jsonl` 写入 + 指数退避回补（对齐 `architecture.md §4.4`）。

### 4.3 验收

- 同 `idempotency_key` 重放返回 duplicate，不重复执行。
- 发生 tool/runner 错误时，事件状态可落到 `failed`。
- `run_mode=NO_REPLY` 的事件不产生用户可见出站。

## 5. Phase 2：查询面与恢复面（M2）

### 5.1 目标

把“可排障、可回放、可恢复”做完整。

### 5.2 交付物

- 事件查询：
  - `GET /v1/events/{event_id}`（含 `delivery_status/detail/outbox_id/run_mode/commit_id`）
  - `GET /v1/events`（过滤 + cursor）
  - `GET /v1/events:lookup?idempotency_key=...`
- 运行查询：`GET /v1/runs`、`GET /v1/runs/{id}`、`GET /v1/runs/{id}/trace?view=&redact=`。
- 会话查询：`GET /v1/sessions`、`GET /v1/sessions/{session_key}`。
- 异步结果获取策略：v0.4 仅提供 `GET /v1/events/{event_id}` 轮询；SSE/WebSocket/Webhook 列为 P1。
- 崩溃恢复：
  - 优先读 `sessions.json`
  - 不可读时扫描 `runtime/sessions/*.jsonl` 重建最小索引
  - 依据 commit marker 丢弃尾部未提交 entries

### 5.3 验收

- 人为制造“半截 batch”（缺 commit marker），重启后能回滚尾部并继续服务。
- `events:lookup` 能从幂等键反查 `event_id`。
- `redact=true` 输出稳定且不泄露原始敏感内容。

## 6. Phase 3：记忆、RAG 与静默内务（M3）

### 6.1 目标

形成“可压缩上下文 + 可召回事实”的闭环。

### 6.2 交付物

- `memory_search` / `memory_get` 工具（含路径白名单与范围限制）。
- daily/curated 记忆写入动作（`WriteMemory`）。
- `payload.type=memory_flush|compaction|cron_fire` 事件处理。
- NO_REPLY 内务回合：
  - outbound 抑制
  - 默认禁止高风险副作用
- compaction 规则：`latest compaction summary + tail entries` 组装上下文。

### 6.3 验收

- “记住我喜欢 Go”后可通过 `memory_search + memory_get` 回答。
- 触发 compaction 后仍可回答关键事实。
- 越界读取（如 `../../`）被拒绝并记录 trace。

## 7. Phase 4：审批与演化（M4）

### 7.1 目标

上线高风险动作治理与最小自我进化能力。

### 7.2 交付物

- 审批接口：
  - `POST /v1/approvals`
  - `POST /v1/approvals/{id}:approve`
  - `POST /v1/approvals/{id}:reject`
  - `GET /v1/approvals`（列表检索）
- 审批回流事件：`approval_granted/approval_rejected`。
- `Patch` 动作执行：
  - `expected_base_hash=sha256(raw_bytes)`
  - `patch_idempotency_key` 幂等
  - rollback 能恢复上一个稳定版本
- 最小演化日报：`workspace/evolution/YYYY-MM-DD.md`（提案/结果/回滚信息）。

### 7.3 验收

- high risk action 必须进入 pending approval。
- approve 重复调用幂等；approve/reject 冲突返回 `409`。
- patch 应用失败不污染目标文件。

## 8. Phase 5：可选硬化（M5，可跳过）

### 8.1 目标

把系统打磨到“可持续迭代、可演示”的工程状态。

### 8.2 交付物

- CI 增量门禁（M5 可选）：`M4 + make test-fault-injection`。
- 关键链路故障注入：
  - adapter 发送失败重试
  - StoreLoop 写失败
  - 进程中断恢复
- 运维脚本：日志采样、ledger 清理、spool 排障。

### 8.3 验收

- PR 未过门禁不可合并。
- 故障注入场景下无重复副作用、无不可恢复脏状态。

## 9. 横切验收清单（每阶段都要过）

- 幂等：inbound/outbound/action 三层键生效。
- 一致性：未 commit 不出站；`runs` 写失败不前移 `sessions.json`。
- 可观测：任意事件可查 `event -> run -> trace -> commit/outbox`。
- 安全：路径边界、权限默认 deny、NO_REPLY 禁用户可见副作用。

## 10. 风险与缓解

1. 接口实现歧义：以 `interfaces.md v0.4` 为唯一协议源，变更先改文档再改代码。  
2. 文件写损坏：所有索引类文件强制 `tmp + fsync + rename`。  
3. 半截提交：commit marker 校验 + 启动恢复丢尾，不做原地修复。  
4. 演化翻车：默认 `auto_apply=false`，必须审批 + 回滚 + smoke 测试。  
5. 并发复杂化：v0.4 坚持 EventLoop 全局串行推进，Runner 仅做慢调用隔离；后续按需演进为 per-session 并行。

## 11. 建议 PR 拆分（可直接执行）

- PR-1：Phase 0（骨架 + StoreLoop 原子写 + workspace init）。
- PR-2：Phase 1（ingest/resolve/commit/outbound 主链路）。
- PR-3：Phase 2（events/runs/sessions 查询 + 恢复机制）。
- PR-4：Phase 3（memory tools + RAG + NO_REPLY + compaction）。
- PR-5：Phase 4（approval + patch + rollback + evolution 日报）。
- PR-6：Phase 5（CI + 故障注入 + 演示脚本）。

## 12. CI/CD 产物映射（按里程碑交付）

### 12.1 M0 必交付

- `Makefile` 或等价脚本：至少包含 3.2 所列统一入口（含 `accept-current`、`accept-m1..m4`、`docs-consistency`、`test-fault-injection`）；非 M0 阶段目标允许占位实现。
- `.github/workflows/ci.yml` 初版：`make fmt`、`make vet`、`make lint`、`make test-unit`、`make test-unit-race-core`。
- `tests/` 目录骨架：`tests/integration`、`tests/e2e`。
- `VERSION_STAGE` 文件与 `make accept-current` 可用（CI 不再写死 `accept-m1`)。

### 12.2 M1 必交付

- Ingest/幂等/提交边界集成测试用例：
  - `POST /v1/events:ingest` 重放去重
  - StoreLoop commit 顺序与“未 ack 不出站”
- `ci.yml` 增加 integration 阶段，入口固定为 `make test-integration`（其内部固定执行 `go test ./tests/integration/... -tags=integration`）。
- PR 门禁必须包含：`make vet`、`make lint`、`make test-unit-race-core`、`make test-e2e-smoke`、`make accept-current`。
- `make test-e2e-smoke` 按 `VERSION_STAGE` 运行增量用例集，M1 仅要求基础链路 smoke。

### 12.3 M2 必交付

- 恢复类测试用例：
  - 半截 batch（缺 commit marker）恢复
  - `sessions.json` 损坏/缺失重建
- 查询接口回归用例：
  - `GET /v1/events`、`GET /v1/events:lookup`、`GET /v1/runs`、`GET /v1/sessions`
- 可选新增 `.github/workflows/nightly-e2e.yml`（main/nightly 跑全量 E2E）。

### 12.4 M3 必交付

- Memory/RAG/NO_REPLY E2E 用例：
  - 记忆写入与召回
  - `memory_get` 越界拒绝
  - `NO_REPLY` 无用户可见出站
- Trace 脱敏回归用例：`redact=true` 输出稳定。

### 12.5 M4 必交付

- Approval/Patch/Rollback 回归套件：
  - 审批幂等与冲突码（`200/409`）
  - `expected_base_hash`、`patch_idempotency_key` 语义
  - rollback 生效验证
- Patch Guard 脚本或测试入口（schema/lint/smoke）。

### 12.6 M5 可选（无发版模式可跳过）

- 故障注入测试常态化（main/nightly）：显式入口 `make test-fault-injection`（覆盖 StoreLoop 写失败、adapter 重试、重启恢复）。
- 演示脚本稳定性回归：2-3 分钟链路每次变更可复现。
- 可选新增 `.github/workflows/nightly-e2e.yml`，不要求 `release.yml`。

## 13. 阶段就绪定义（DoD）

- 文档一致：`requirements/architecture/interfaces/roadmap/cicd-testing` 术语一致，无旧并发 worker 与旧数据库术语残留。  
- 功能一致：M1-M4 验收脚本全部通过。  
- 恢复一致：随机中断后可恢复且不重复副作用。  
- 演示一致：2-3 分钟脚本稳定复现“记忆召回 + 审批 + 回滚”闭环。
