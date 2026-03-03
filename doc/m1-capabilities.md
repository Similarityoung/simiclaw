# M1 当前能力说明

本文描述仓库在 `M1` 阶段已经实现并可用的能力边界，作为「当前可交付行为」说明。

## 1. 阶段定位

- 当前阶段：`M1`（由根目录 `VERSION_STAGE` 驱动）
- 目标链路：`ingest -> EventLoop -> AgentRun -> StoreLoop commit -> outbound`
- 设计取舍：正确性优先，单机单进程，EventLoop 全局串行推进

## 2. 已实现能力

### 2.1 HTTP 接口

- `POST /v1/events:ingest`
- `GET /v1/events/{event_id}`
- `GET /healthz`
- `GET /readyz`

说明：

- 认证：当 `api_key` 配置为空时不校验；非空时要求 `Authorization: Bearer <API_KEY>`。
- 返回体和错误码遵循 `doc/interfaces.md` 的 v0.4 约定。

### 2.2 入站校验与幂等

- 必填字段校验：`source/conversation/payload.type/idempotency_key/timestamp`
- `timestamp` 必须是 UTC，且与服务端时间漂移不超过 10 分钟
- 幂等键格式：
  - Telegram：`telegram:update:<update_id>`
  - CLI：`cli:<conversation_id>:<seq>`
- 入站幂等行为：
  - 同 key + 同 payload：返回 duplicate（不重复执行）
  - 同 key + 不同 payload：返回 `409 CONFLICT`

### 2.3 会话路由与会话索引

- 基于 `tenant_id + conversation + dm_scope` 计算 `session_key`
- `ResolveSession` 将 `session_key` 绑定到当前 `active_session_id`
- 首次会话会创建 `runtime/sessions/<session_id>.jsonl` header 与 `sessions.json` 索引

### 2.4 事件执行与提交一致性

- EventLoop 串行消费 `InternalEvent`
- Runner 产生会话 entries 与 run trace
- StoreLoop 提交顺序固定：
  1. AppendBatch 到 `runtime/sessions/<session_id>.jsonl`
  2. 写 `runtime/runs/<run_id>.json`
  3. 更新 `runtime/sessions.json`
- 只有收到 StoreLoop commit 成功后，才允许 outbound 发送

### 2.5 出站与 NO_REPLY

- 正常回合：尝试直接发送（当前默认 sender 为 `StdoutSender`）
- 发送失败：落盘到 `runtime/outbound_spool/<outbox_id>.json`
- `payload.type=memory_flush|compaction|cron_fire` 时，run mode 为 `NO_REPLY`，抑制用户可见出站

## 3. 运行状态模型

### 3.1 Event 状态

- `accepted`
- `running`
- `committed`
- `failed`

### 3.2 Delivery 状态

- `not_applicable`
- `pending`
- `sent`
- `suppressed`
- `failed`

典型成功路径：

- `accepted -> running -> committed`
- `delivery_status: not_applicable -> pending -> sent`

## 4. 运行时文件布局

以 `<workspace>` 为根目录：

- `runtime/events/events.json`：事件状态仓库
- `runtime/sessions.json`：会话索引
- `runtime/sessions/<session_id>.jsonl`：会话事实源（entry + commit marker）
- `runtime/runs/<run_id>.json`：单次运行 trace
- `runtime/idempotency/inbound_keys.jsonl`：入站幂等账本
- `runtime/idempotency/outbound_keys.jsonl`：出站幂等账本
- `runtime/outbound_spool/*.json`：出站失败暂存

## 5. Telegram 在 M1 的范围

M1 已实现 Telegram 协议归一化与测试夹具回放，不包含真实 Telegram long polling 生产接入。

## 6. 验收与测试入口

关键命令：

- `make test-integration`
- `make test-e2e-smoke`
- `make accept-m1`
- `make accept-current`

M1 验收重点：

- 同 `idempotency_key` 重放返回 duplicate，不重复执行
- commit 顺序满足 `append -> run -> sessions`
- `NO_REPLY` 不产生用户可见出站

## 7. 非 M1 范围（后续阶段）

以下能力不属于 M1 完整交付范围：

- `GET /v1/events`、`events:lookup`、`runs/sessions` 查询面（M2）
- 崩溃恢复完整机制（M2）
- memory_search/memory_get 与 compaction 闭环（M3）
- approvals/patch/rollback（M4）
