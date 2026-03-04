# M2 当前能力说明

本文描述仓库在 `M2` 阶段已经实现并可用的能力边界，作为「当前可交付行为」说明。

## 1. 阶段定位

- 当前阶段：`M2`（由根目录 `VERSION_STAGE` 驱动）
- 目标能力：在 M1 主链路稳定前提下补齐查询面与恢复面
- 设计取舍：单机单进程、文件持久化、正确性与可恢复优先

## 2. 已实现能力

### 2.1 HTTP 接口

- 入站：`POST /v1/events:ingest`
- 健康：`GET /healthz`、`GET /readyz`
- 事件查询：
  - `GET /v1/events/{event_id}`
  - `GET /v1/events`
  - `GET /v1/events:lookup?idempotency_key=...`
- 运行查询：
  - `GET /v1/runs`
  - `GET /v1/runs/{run_id}`
  - `GET /v1/runs/{run_id}/trace?view=&redact=`
- 会话查询：
  - `GET /v1/sessions`
  - `GET /v1/sessions/{session_key}`

### 2.2 查询面能力

- `events` 列表支持过滤：`conversation_id/session_key/status/delivery_status`。
- `runs` 列表支持过滤：`conversation_id/session_key/session_id`。
- `sessions` 列表支持过滤：`conversation_id/session_key`。
- 列表接口统一支持 `limit/cursor` 分页；非法 cursor 返回 `400 INVALID_ARGUMENT`。
- `runs/{id}/trace` 支持 `view=full|summary` 与 `redact=true|false`。

### 2.3 会话索引扩展字段

`runtime/sessions.json` 的行结构扩展并落盘：

- `conversation_id`
- `channel_type`
- `participant_id`
- `dm_scope`
- `last_commit_id`
- `last_run_id`

兼容性：历史 `sessions.json` 缺失上述字段时仍可读取并在后续写入时补齐。

### 2.4 崩溃恢复能力

启动时执行恢复流程：

1. 优先读取 `runtime/sessions.json`。
2. 当 `sessions.json` 不可读/缺失/为空时，扫描 `runtime/sessions/*.jsonl` 重建最小索引并原子写回。
3. 对每个 session 文件执行尾部校验：
   - batch 缺 `commit marker`，丢弃未提交尾部
   - `entry_count/last_entry_id/run_id` 校验失败，回滚到最后一个合法 marker
4. 恢复完成后服务可继续 ingest/commit/query。

## 3. 运行状态与一致性

- Event 状态：`accepted -> running -> committed|failed`
- Delivery 状态：`not_applicable|pending|sent|suppressed|failed`
- StoreLoop 提交顺序仍固定：
  1. append `sessions/<session_id>.jsonl`
  2. write `runs/<run_id>.json`
  3. update `sessions.json`

## 4. 验收入口

关键命令：

- `make test-integration`
- `make test-e2e-smoke`
- `make accept-m2`
- `make accept-current`（当 `VERSION_STAGE=M2`）

M2 验收重点：

- 查询面接口可用并可分页
- `events:lookup` 可由幂等键反查 `event_id`
- `sessions.json` 损坏/缺失可重建
- 半截 batch 恢复时可丢尾并继续处理新事件

## 5. 非 M2 范围（后续阶段）

以下能力不属于 M2 完整交付范围：

- memory_search/memory_get 与 compaction 闭环（M3）
- approvals/patch/rollback（M4）
- Telegram long polling 生产接入（后续回补）
