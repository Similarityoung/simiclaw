# M4 当前能力说明

本文描述仓库在 `M4` 阶段已经实现并可用的能力边界，作为「当前可交付行为」说明。

## 1. 阶段定位

- 当前阶段：`M4`（由根目录 `VERSION_STAGE` 驱动）
- 目标能力：在 M3 的记忆/RAG/NO_REPLY 基础上补齐审批闭环与 patch apply/rollback。

## 2. 已实现能力

### 2.1 审批 API

- `POST /v1/approvals`
- `POST /v1/approvals/{approval_id}:approve`
- `POST /v1/approvals/{approval_id}:reject`
- `GET /v1/approvals/{approval_id}`
- `GET /v1/approvals`

状态机：

- `pending -> approved|rejected|expired`
- 同一决策重复调用幂等返回 `200`
- 相反决策冲突返回 `409`

### 2.2 高风险动作自动审批

- runner 支持 `/patch` 指令生成 `Patch` 高风险动作（`requires_approval=true`）。
- EventLoop 识别高风险动作后自动创建 pending approval。
- 在审批通过前不执行 patch，先返回“已进入审批队列”。

### 2.3 审批回流与执行

- approve/reject 成功后发布 `approval_granted/approval_rejected` 内部事件。
- `approval_granted` 回合执行待审批 patch 并写回 run trace。
- `approval_rejected` 回合仅记录并回执“未执行变更”。

### 2.4 Patch apply 与自动回滚

- 仅允许 `workflows/**`、`skills/**` 目标路径。
- 校验 `expected_base_hash=sha256(raw_bytes_of_target_file)`。
- 支持单文件 `unified-diff`。
- `patch_idempotency_key` 重复返回首次结果（成功或失败）。
- apply 失败或 guard 失败时自动回滚，保证目标文件不被污染。

### 2.5 Evolution 日报

- 审批创建、审批决策、审批执行结果均追加到：
  - `workspace/evolution/YYYY-MM-DD.md`

## 3. 验收入口

- `make test-unit`
- `make test-integration`
- `make test-e2e-smoke`
- `make test-patch-guard`
- `make accept-m4`
- `make accept-current`（当 `VERSION_STAGE=M4`）

## 4. 非 M4 范围（后续阶段）

- 手动 rollback HTTP 接口（当前仅支持 apply 失败自动回滚）。
- 多文件 patch / rename / binary patch 支持。
- 更强 patch guard（例如外部 schema/lint/smoke 全量管线）。
