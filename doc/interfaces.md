# 接口文档 v0.4

> 本文定义系统对外/对内核心接口：入站事件、事件状态查询、审批回流、会话查询、Cron 管理、运行追踪、Tool/Skill Runner 协议。  
> 对齐 [architecture.md](./architecture.md) 的 `Gateway + EventLoop + StoreLoop + sessions.json + <session_id>.jsonl` 模型。

## 0. 约定与通用规范

### 0.1 基础约定

- Base URL：`http://localhost:8080`
- Content-Type：`application/json; charset=utf-8`
- 时间：ISO8601 UTC（请求 `timestamp` 必填）
- ID：UUIDv7/UUIDv4 或可排序短 ID（全局唯一）
- 服务端接收时间统一记录为 `received_at`

### 0.2 认证与鉴权

v0.4 对外入口支持两类认证方式，并保留本地 CLI 信任模式：

1. Header API Key：`Authorization: Bearer <API_KEY>`
2. Telegram Bot 鉴权：bot token + `chat_id/user_id` allowlist（long polling）
3. 本地 CLI 直连模式（仅本机）：不走 HTTP 鉴权头，依赖本地进程信任边界

服务端注入：

- `tenant_id`（v0.4 固定 `local`）
- `scopes`

Telegram 入口若 `chat_id/user_id` 不在 allowlist，返回 `403 FORBIDDEN`。

### 0.3 幂等语义

- 入站幂等：`idempotency_key`（`runtime/idempotency/inbound_keys.jsonl`）
- 出站幂等：`outbound_idempotency_key`（`runtime/idempotency/outbound_keys.jsonl`）
- 副作用幂等：`action_idempotency_key=run_id:action_index`（`runtime/idempotency/action_keys.jsonl`）
- 若请求在“事件持久化前”或“成功入队前”失败，服务端会回滚该次入站幂等登记；客户端可继续使用同一 `idempotency_key` 重试

### 0.4 会话标识语义

- `session_key`：会话域路由键（计算规则引用 [requirements.md §3.2](./requirements.md) `SK-FORMULA-001`）
- `active_session_id`：当前活跃会话 ID（即当前 `session_id`）
- `dm_scope`：v0.4 枚举仅 `default`（默认值）；字段预留用于未来区分同一 DM 对话的上下文域（如 `work/personal`）

`session_key` 规范化规则：

- 组件：`tenant_id`, `conversation_id`, `channel_type`, `participant_id_or_dash`, `dm_scope`
- `participant_id_or_dash` 规则：`channel_type=dm` 时取 `participant_id`；否则固定为 `"-"`
- `thread_id` 不参与 `session_key` 计算（v0.4 决策）；仅用于 UI 展示与 trace 辅助信息，因此同 `conversation_id` 的不同 thread 共享同一会话上下文
- 计算细节（拼接顺序、哈希格式）按 `SK-FORMULA-001` 执行

`ResolveSession`：

1. 无 `session_key` 记录：创建新 `session_id`，写 jsonl header，更新 `sessions.json`
2. 命中轮换条件（reset/idle/daily）：创建新 `session_id`，旧会话追加 `session_rotate`，更新 `sessions.json`
3. 否则返回已有 `active_session_id`

### 0.5 统一错误格式

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "field conversation.conversation_id is required",
    "details": {
      "field": "conversation.conversation_id"
    }
  }
}
```

常见 `error.code`：

- `UNAUTHORIZED`
- `FORBIDDEN`
- `INVALID_ARGUMENT`
- `NOT_FOUND`
- `CONFLICT`
- `RATE_LIMITED`
- `QUEUE_FULL`
- `QUEUE_UNAVAILABLE`
- `CANCELED`
- `INTERNAL`

说明：

- `RATE_LIMITED` 对应速率限制，HTTP 返回 `429`，应带 `Retry-After`。
- `QUEUE_FULL` 对应 EventLoop 背压，HTTP 返回 `503`，应带 `Retry-After`。
- `QUEUE_UNAVAILABLE` 表示队列已关闭/不可用，HTTP 返回 `503`，应带 `Retry-After`。
- `CANCELED` 表示请求在服务端处理过程中被客户端取消，HTTP 返回 `499`（非标准码）。

### 0.6 限流契约（P0）

- 算法：token bucket。
- 维度：
  - `tenant_id`（全局入口保护）
  - `session_key`（单会话热点保护）
- 默认阈值（可配置）：
  - `tenant_id`：`30 req/s, burst=60`
  - `session_key`：`5 req/s, burst=10`
- 返回约定：
  - 超限返回 `429 RATE_LIMITED`
  - Header 必须包含 `Retry-After`（秒）
  - 建议附带 `X-RateLimit-Limit`、`X-RateLimit-Remaining`、`X-RateLimit-Reset`

## 1. 数据结构定义（核心）

### 1.1 InternalEvent

```json
{
  "event_id": "evt_...",
  "source": "cli|telegram|cron|approval|system",
  "tenant_id": "local",
  "scopes": ["chat:write", "memory:read", "memory:write"],
  "conversation": {
    "conversation_id": "conv_001",
    "thread_id": "th_01",
    "channel_type": "dm|group|channel",
    "participant_id": "user_123"
  },
  "session_key": "sk:9f5a3b7c...",
  "idempotency_key": "telegram:update:123456789",
  "timestamp": "2026-03-03T12:00:00Z",
  "payload": {
    "type": "message|button|cron_fire|approval_granted|approval_rejected|memory_flush|compaction",
    "text": "hello",
    "mentions": ["@bot"],
    "attachments": [
      {
        "attachment_id": "att_...",
        "filename": "a.pdf",
        "mime": "application/pdf",
        "size": 12345
      }
    ],
    "native": {
      "update_id": 123456789
    },
    "native_ref": "runtime/native/telegram/123456789.json"
  }
}
```

最小必填字段（请求侧）：

- `source`
- `conversation.conversation_id`
- `conversation.channel_type`
- `payload.type`
- `idempotency_key`
- `timestamp`

条件必填：

- `conversation.channel_type=dm` 时，`conversation.participant_id` 必填

说明：

- ingest request 不要求客户端提供 `session_key`，服务端计算并回填
- `payload.native` 可为对象；超大原始数据建议使用 `native_ref` 引用落盘路径
- `native_ref` 仅允许 `runtime/native/**` 下相对路径；禁止绝对路径与 `..` 跳转路径
- 同时存在 `native` 与 `native_ref` 时，以服务端最终落盘后的 `native_ref` 为准
- 客户端传入 `native_ref` 仅作 hint，服务端可覆盖为安全路径
- native 原始数据落盘由服务端负责，客户端不应依赖自定义落盘路径生效
- `attachments` 在 v0.4 仅作为元信息，不提供附件内容读取协议
- 附件内容读取能力留到后续版本（计划由专用 tool 或附件接口承载）
- `conversation.channel_type=channel` 在 v0.4 为保留枚举（reserved），当前不接入频道写回链路；若外部请求显式传入该值，建议返回 `400 INVALID_ARGUMENT`

幂等键规范：

- `source=telegram`：`idempotency_key=telegram:update:<update_id>`
- `source=cli`：`idempotency_key=cli:<conversation_id>:<seq>`
- 同一上游事件重试必须复用相同 key

### 1.2 事件 payload schema（定版）

`payload.type=approval_granted` 时：

```json
{
  "type": "approval_granted",
  "approval_id": "apr_01H...",
  "actor": {
    "type": "user",
    "id": "user_123"
  },
  "note": "OK",
  "actions": ["act_1", "act_2"]
}
```

`payload.type=approval_rejected` 时，结构与 `approval_granted` 相同（不再重复 `decision` 字段）。

`payload.type=cron_fire` 时：

```json
{
  "type": "cron_fire",
  "job_id": "daily_memory_flush",
  "payload_type": "memory_flush"
}
```

`payload.type=memory_flush|compaction` 时：

```json
{
  "type": "memory_flush",
  "reason": "threshold|manual|cron",
  "trigger_job_id": "daily_memory_flush"
}
```

### 1.3 Action（意图层）

```json
{
  "action_id": "act_...",
  "action_index": 0,
  "action_idempotency_key": "run_01H...:0",
  "type": "SendMessage|Invoke|WriteMemory|Patch|RequestApproval",
  "risk": "low|medium|high",
  "requires_approval": false,
  "payload": {}
}
```

`type` 语义：

- `SendMessage`：发送消息
- `Invoke`：调用 tool/skill（`payload.target=tool|skill`, `payload.name`, `payload.args`）
- `WriteMemory`：写记忆（`payload.target=daily|curated`）
- `Patch`：变更 skill/workflow（`payload.target=skill|workflow`）
- `RequestApproval`：创建审批

### 1.4 ActionPlan

```json
{
  "run_id": "run_...",
  "session_key": "sk:9f5a3b7c...",
  "active_session_id": "s_20260303_001",
  "run_mode": "NORMAL|NO_REPLY",
  "actions": [
    {
      "action_id": "act_1",
      "action_index": 0,
      "action_idempotency_key": "run_...:0",
      "type": "SendMessage",
      "risk": "low",
      "payload": {}
    }
  ]
}
```

### 1.5 RunTrace（高层）

```json
{
  "run_id": "run_...",
  "event_id": "evt_...",
  "session_key": "sk:9f5a3b7c...",
  "session_id": "s_20260303_001",
  "run_mode": "NORMAL|NO_REPLY",
  "status": "running|succeeded|failed",
  "client_timestamp": "2026-03-03T12:00:00Z",
  "received_at": "2026-03-03T12:00:00.120Z",
  "started_at": "2026-03-03T12:00:01Z",
  "ended_at": "2026-03-03T12:00:05Z",
  "context_manifest": {
    "history_range": {
      "mode": "compaction+tail",
      "cutoff_commit_id": "c_100",
      "tail_limit": 200
    }
  },
  "commit_summary": {
    "commit_id": "c_101",
    "entry_count": 4,
    "last_entry_id": "e4"
  },
  "errors": []
}
```

### 1.6 Session JSONL Entry（事实源）

每个 `runtime/sessions/<session_id>.jsonl`：

- 第一条必须是 `type=header`
- 每个 run 的最后一条必须是 `type=commit`
- `commit.entry_count` 定义为“本次 batch 中不含 commit 自身的 entry 数量”

示例：

```json
{"type":"header","session_id":"s_20260303_001","session_key":"sk:9f5a3b7c...","created_at":"2026-03-03T10:00:00Z","format_version":"1"}
{"type":"user","entry_id":"e1","run_id":"r1","content":"你好"}
{"type":"assistant","entry_id":"e2","run_id":"r1","tool_calls":[{"tool_call_id":"tc1","name":"memory_search"}]}
{"type":"tool_result","entry_id":"e3","run_id":"r1","tool_call_id":"tc1","ok":true,"result":"..."}
{"type":"assistant","entry_id":"e4","run_id":"r1","content":"已完成"}
{"type":"commit","entry_id":"e5","run_id":"r1","commit_id":"c1","entry_count":4,"last_entry_id":"e4"}
```

## 2. Gateway：入站事件 API

v0.4 结果获取仅定义 HTTP 轮询（`GET /v1/events/{event_id}`）；SSE/WebSocket/Webhook 为 P1 扩展，不在当前协议范围。

### 2.1 POST `/v1/events:ingest`

用于 CLI/Telegram/Cron/Approval 投递 `InternalEvent`。

Request：

```json
{
  "source": "telegram",
  "conversation": {
    "conversation_id": "conv_001",
    "channel_type": "dm",
    "participant_id": "user_123"
  },
  "idempotency_key": "telegram:update:123456789",
  "timestamp": "2026-03-03T12:00:00Z",
  "payload": {
    "type": "message",
    "text": "帮我记住我喜欢 Go",
    "native": {
      "update_id": 123456789
    }
  }
}
```

约束：

- `idempotency_key` 必填，且重试时必须不变
- `timestamp` 必填且必须是 UTC
- 默认时钟漂移窗口：`abs(now - timestamp) <= 10m`，超出返回 `400 INVALID_ARGUMENT`
- `source=telegram` 时，`idempotency_key` 匹配 `telegram:update:<update_id>`
- `source=cli` 时，`idempotency_key` 匹配 `cli:<conversation_id>:<seq>`
- `session_key` 如由客户端传入，仅作参考，服务端可覆盖
- `native_ref` 若出现，必须满足相对路径且位于 `runtime/native/**`（否则 `400 INVALID_ARGUMENT`）
- `received_at` 由服务端生成；客户端请求若携带该字段一律忽略
- Gateway 必须执行有界队列背压：超过 `ingest_enqueue_timeout`（默认 `200ms`）仍无法入队时返回 `503 SERVICE_UNAVAILABLE`（`error.code=QUEUE_FULL`）

Response（`202 Accepted`）：

```json
{
  "event_id": "evt_01H...",
  "session_key": "sk:9f5a3b7c...",
  "active_session_id": "s_20260303_001",
  "received_at": "2026-03-03T12:00:00.120Z",
  "payload_hash": "sha256:9f5a...",
  "status": "accepted",
  "status_url": "/v1/events/evt_01H..."
}
```

`202 Accepted` 语义保证：

- 返回 `202` 前，服务端至少已持久化最小事件元信息：`event_id`, `idempotency_key`, `payload_hash`, `received_at`
- 崩溃恢复后应能基于上述元信息继续查询 `GET /v1/events/{event_id}` 或执行幂等判定

Duplicate Response（`200 OK`）：

```json
{
  "event_id": "evt_existing",
  "session_key": "sk:9f5a3b7c...",
  "active_session_id": "s_20260303_001",
  "received_at": "2026-03-03T12:00:00.120Z",
  "payload_hash": "sha256:9f5a...",
  "status": "duplicate_acked",
  "status_url": "/v1/events/evt_existing"
}
```

错误：

- `400 INVALID_ARGUMENT`：缺字段、时间非法、幂等键格式非法
- `409 CONFLICT`：同 `idempotency_key` 但 canonical payload hash 不一致
- `429 RATE_LIMITED`：触发限流（附 `Retry-After`）
- `503 SERVICE_UNAVAILABLE`：事件队列满或入队超时（`error.code=QUEUE_FULL`，附 `Retry-After`）

`409 CONFLICT` 详情示例：

```json
{
  "error": {
    "code": "CONFLICT",
    "message": "idempotency payload hash mismatch",
    "details": {
      "expected_hash": "sha256:9f5a...",
      "got_hash": "sha256:a81c..."
    }
  }
}
```

### 2.2 GET `/v1/events/{event_id}`

查询事件处理状态（用于 CLI/WebUI 轮询）。

Response：

```json
{
  "event_id": "evt_01H...",
  "status": "accepted|running|committed|failed",
  "delivery_status": "not_applicable|pending|sent|suppressed|failed",
  "delivery_detail": "direct|spooled|not_applicable",
  "outbox_id": "out_01H...",
  "session_key": "sk:9f5a3b7c...",
  "session_id": "s_20260303_001",
  "run_id": "run_01H...",
  "run_mode": "NORMAL|NO_REPLY",
  "commit_id": "c_101",
  "assistant_reply": "已收到: hello",
  "received_at": "2026-03-03T12:00:00.120Z",
  "error": null,
  "updated_at": "2026-03-03T12:00:03Z"
}
```

失败示例：

```json
{
  "event_id": "evt_01H...",
  "status": "failed",
  "delivery_status": "failed",
  "delivery_detail": "spooled",
  "outbox_id": "out_01H...",
  "session_key": "sk:9f5a3b7c...",
  "session_id": "s_20260303_001",
  "run_id": "run_01H...",
  "run_mode": "NORMAL",
  "commit_id": "c_101",
  "received_at": "2026-03-03T12:00:00.120Z",
  "error": {
    "code": "INTERNAL",
    "message": "tool runner timeout"
  },
  "updated_at": "2026-03-03T12:00:30Z"
}
```

语义约定：

- `status` 表示执行阶段：`accepted|running|committed|failed`
- `delivery_status` 表示投递阶段
- `delivery_detail` 表示投递路径：`direct`（直发）或 `spooled`（进入重试队列）
- `status=committed` 仅表示 StoreLoop commit 成功（jsonl + runtrace + sessions.json）
- `delivery_status=sent` 仅在 adapter 返回成功（或 spool 标记 sent）后成立
- `run_mode=NO_REPLY` 时，`delivery_status=suppressed`
- `assistant_reply` 为本次回合生成的最终文本回复；`run_mode=NO_REPLY` 或无文本回复时可省略
- `delivery_status=not_applicable` 用于“尚未进入投递阶段”的事件（通常 `status=accepted|running`），或“该事件本身不产生出站动作”的场景；一旦进入投递阶段应转换为 `pending|sent|suppressed|failed`
- 字段组合约束：`delivery_status=not_applicable` 时，`delivery_detail=not_applicable`，且 `outbox_id` 必须为 `null` 或省略
- 字段组合约束：`delivery_detail=direct` 时，`outbox_id` 必须为 `null` 或省略
- 字段组合约束：`delivery_detail=spooled` 时，`outbox_id` 必填

错误：

- `404 NOT_FOUND`：`event_id` 不存在

### 2.3 GET `/v1/events`

事件列表查询（可选，便于运维排障）。

Query params：

- `conversation_id=...`
- `session_key=...`
- `status=accepted|running|committed|failed`
- `delivery_status=pending|sent|suppressed|failed`
- `limit=50`
- `cursor=...`（base64 token）

`cursor` 规范：

- token 至少包含：`last_updated_at,last_event_id`
- 非法 cursor 返回 `400 INVALID_ARGUMENT`

Response：

```json
{
  "items": [
    {
      "event_id": "evt_01H...",
      "status": "committed",
      "delivery_status": "pending",
      "session_key": "sk:9f5a3b7c...",
      "session_id": "s_20260303_001",
      "run_id": "run_01H...",
      "updated_at": "2026-03-03T12:00:03Z"
    }
  ],
  "next_cursor": "YmFzZTY0X3Rva2Vu"
}
```

### 2.4 GET `/v1/events:lookup`

按幂等键查询事件（便于排障）。

Query params：

- `idempotency_key=...`

Response：

```json
{
  "event_id": "evt_01H...",
  "payload_hash": "sha256:9f5a...",
  "received_at": "2026-03-03T12:00:00.120Z",
  "status_url": "/v1/events/evt_01H..."
}
```

错误：

- `404 NOT_FOUND`：未找到对应幂等键

### 2.5 GET `/healthz`

进程存活探针（liveness）。

Response（`200 OK`）：

```json
{
  "status": "ok",
  "time": "2026-03-03T12:00:00Z"
}
```

### 2.6 GET `/readyz`

就绪探针（readiness），用于容器或负载均衡摘流判断。

返回约定：

- `200 OK`：可接收新请求
- `503 SERVICE_UNAVAILABLE`：未就绪（例如 StoreLoop 初始化失败、关键目录不可写、关键 worker 未就绪）

未就绪示例：

```json
{
  "status": "not_ready",
  "reasons": ["storeloop_not_ready", "workspace_not_writable"]
}
```

## 3. Approval：审批 API

### 3.1 POST `/v1/approvals`

创建审批（通常由 Policy Gate 内部调用，也可对外暴露）。

Request：

```json
{
  "run_id": "run_01H...",
  "session_key": "sk:9f5a3b7c...",
  "active_session_id": "s_20260303_001",
  "conversation_id": "conv_001",
  "expires_at": "2026-03-04T00:00:00Z",
  "summary": "需要修改 workflow",
  "risk": "high",
  "actions": [
    {
      "action_id": "act_1",
      "action_index": 2,
      "action_idempotency_key": "run_01H...:2",
      "type": "Patch",
      "payload": {
        "target": "workflow",
        "target_path": "workflows/curate_memory.yaml",
        "patch_format": "unified-diff",
        "diff": "--- a/... \n+++ b/...\n@@ ...",
        "expected_base_hash": "sha256:abc123...",
        "patch_idempotency_key": "run_01H...:act_patch_1"
      }
    }
  ]
}
```

Response（`201 Created`）：

```json
{
  "approval_id": "apr_01H...",
  "status": "pending",
  "approve_url": "http://localhost:8080/v1/approvals/apr_01H...:approve",
  "reject_url": "http://localhost:8080/v1/approvals/apr_01H...:reject"
}
```

### 3.2 POST `/v1/approvals/{approval_id}:approve`

Request：

```json
{
  "actor": {
    "type": "user",
    "id": "user_123"
  },
  "note": "OK"
}
```

Response：

```json
{
  "approval_id": "apr_01H...",
  "status": "approved"
}
```

副作用：生成 `InternalEvent(payload.type=approval_granted)` 回流到同 `session_key`。

返回码约定：

- 首次 approve 成功：`200 OK`
- 重复 approve：`200 OK`（返回相同状态）
- 已 reject 后再 approve：`409 CONFLICT`
- 已过期再 approve：`409 CONFLICT`

### 3.3 POST `/v1/approvals/{approval_id}:reject`

与 approve 对称，状态置为 `rejected`，回流 `payload.type=approval_rejected`。

返回码约定：

- 首次 reject 成功：`200 OK`
- 重复 reject：`200 OK`（返回相同状态）
- 已 approve 后再 reject：`409 CONFLICT`
- 已过期再 reject：`409 CONFLICT`

### 3.4 GET `/v1/approvals/{approval_id}`

返回审批详情。

Response：

```json
{
  "approval_id": "apr_01H...",
  "status": "pending|approved|rejected|expired",
  "risk": "low|medium|high",
  "session_key": "sk:9f5a3b7c...",
  "session_id": "s_20260303_001",
  "run_id": "run_01H...",
  "conversation_id": "conv_001",
  "summary": "需要修改 workflow",
  "actions": [
    {
      "action_id": "act_1",
      "action_index": 2,
      "type": "Patch"
    }
  ],
  "created_at": "2026-03-03T12:00:00Z",
  "expires_at": "2026-03-04T00:00:00Z",
  "updated_at": "2026-03-03T12:00:00Z",
  "decision": {
    "actor": {
      "type": "user",
      "id": "user_123"
    },
    "note": "OK",
    "decided_at": "2026-03-03T12:05:00Z"
  }
}
```

说明：

- `status=pending` 时，`decision` 应为 `null` 或省略。

### 3.5 GET `/v1/approvals`

审批列表查询（用于查看所有 `pending/approved/rejected/expired` 审批）。

Query params：

- `status=pending|approved|rejected|expired`
- `conversation_id=...`
- `session_key=...`
- `run_id=...`
- `limit=50`
- `cursor=...`

Response：

```json
{
  "items": [
    {
      "approval_id": "apr_01H...",
      "status": "pending",
      "risk": "high",
      "session_key": "sk:9f5a3b7c...",
      "session_id": "s_20260303_001",
      "run_id": "run_01H...",
      "summary": "需要修改 workflow",
      "created_at": "2026-03-03T12:00:00Z",
      "expires_at": "2026-03-04T00:00:00Z",
      "updated_at": "2026-03-03T12:00:00Z"
    }
  ],
  "next_cursor": "YmFzZTY0X3Rva2Vu"
}
```

## 4. Runs & Traces：运行查询接口

### 4.1 GET `/v1/runs`

支持按 `conversation_id/session_key/session_id/time range` 过滤。

Query params：

- `conversation_id=...`
- `session_key=...`
- `session_id=...`
- `limit=50`
- `cursor=...`

`cursor` 规范：

- `cursor` 为 base64 编码 token
- token 至少包含：`last_run_id,last_started_at`
- 非法 cursor 返回 `400 INVALID_ARGUMENT`

Response：

```json
{
  "items": [
    {
      "run_id": "run_01H...",
      "event_id": "evt_...",
      "session_key": "sk:9f5a3b7c...",
      "session_id": "s_20260303_001",
      "run_mode": "NORMAL",
      "status": "succeeded",
      "started_at": "...",
      "ended_at": "..."
    }
  ],
  "next_cursor": "YmFzZTY0X3Rva2Vu"
}
```

### 4.2 GET `/v1/runs/{run_id}`

返回 run 概览（不含完整 trace）。

### 4.3 GET `/v1/runs/{run_id}/trace`

返回完整 RunTrace（调试/回放）。

Query params（可选）：

- `view=full|summary`（默认 `full`）
- `redact=true|false`（默认 `false`）

`redact=true` 最小规则：

- `patch.diff` 返回 `<redacted>`
- tool 原始 args 不回传，仅保留 `args_summary`
- `memory_get.content` 截断或置为 `<redacted>`
- `native/native_ref` 不回传原文，仅保留必要标识

## 5. Sessions：会话查询接口

### 5.1 GET `/v1/sessions`

支持按 `conversation_id/session_key` 过滤。

Query params：

- `conversation_id=...`
- `session_key=...`
- `limit=50`
- `cursor=...`（base64 token）

Response：

```json
{
  "items": [
    {
      "session_key": "sk:9f5a3b7c...",
      "active_session_id": "s_20260303_001",
      "conversation_id": "conv_001",
      "channel_type": "dm",
      "participant_id": "user_123",
      "dm_scope": "default",
      "updated_at": "2026-03-03T12:00:03Z",
      "last_commit_id": "c_101",
      "last_run_id": "run_01H..."
    }
  ],
  "next_cursor": "YmFzZTY0X3Rva2Vu"
}
```

说明：

- `conversation_id/channel_type/participant_id/dm_scope` 为 `sessions.json` 的索引字段持久化值
- 这些字段不通过 `session_key` 反推得到，避免哈希不可逆导致的歧义

### 5.2 GET `/v1/sessions/{session_key}`

返回单会话索引信息。

## 6. Cron：任务配置与触发接口（可选）

`runtime/cron/jobs.json` 为持久化源，接口通过 StoreLoop 原子读写。

### 6.1 GET `/v1/cron/jobs`

Response：

```json
{
  "version": 12,
  "etag": "W/\"jobs-v12\"",
  "jobs": [
    {
      "job_id": "daily_memory_flush",
      "cron": "0 0 * * *",
      "enabled": true,
      "target": {
        "session_key": "sk:9f5a3b7c...",
        "payload_type": "memory_flush"
      },
      "last_fired_at": "2026-03-03T00:00:00Z",
      "next_fire_at": "2026-03-04T00:00:00Z",
      "last_error": null
    }
  ]
}
```

### 6.2 PUT `/v1/cron/jobs`

Request：

```json
{
  "if_match_version": 12,
  "jobs": [
    {
      "job_id": "daily_memory_flush",
      "cron": "0 0 * * *",
      "enabled": true,
      "target": {
        "session_key": "sk:9f5a3b7c...",
        "payload_type": "memory_flush"
      }
    }
  ]
}
```

约束：

- `if_match_version` 必填；与当前版本不一致返回 `409 CONFLICT`
- 写入走 `tmp + fsync + rename`
- 运行时可能写回，运维建议停机修改后再启动
- 运行时写回的 `last_fired_at/next_fire_at/last_error` 不视为配置变更，不触发 version 递增
- 并发写策略：PUT 覆盖配置字段，保留当前 runtime 字段并在写回时合并

### 6.3 POST `/v1/cron/jobs/{job_id}:fire`

只生成 `InternalEvent(source=cron, payload.type=cron_fire)`，不直接改写会话文件。

## 7. Outbound：出站内部契约

### 7.1 提交前置条件

- 必须在 StoreLoop commit ack 成功后发送
- 必须携带 `outbound_idempotency_key`
- `run_mode=NO_REPLY` 默认禁止任何用户可见消息（含 typing/草稿/状态提示）

### 7.2 OutboxEntry（spool 记录格式）

```json
{
  "outbox_id": "out_01H...",
  "outbound_idempotency_key": "run_01H...:act_0",
  "adapter": "telegram",
  "payload": {
    "conversation_id": "conv_001",
    "text": "好的"
  },
  "created_at": "2026-03-03T12:00:01Z",
  "last_attempt_at": "2026-03-03T12:00:30Z",
  "retry_count": 0,
  "max_retries": 8,
  "next_attempt_at": "2026-03-03T12:01:00Z",
  "last_error": null
}
```

### 7.3 出站失败处理

- 失败必须写 `runtime/outbound_spool/pending.jsonl`
- 重试时必须复用同一 `outbound_idempotency_key`
- 同一 `outbound_idempotency_key` 仅允许一个活跃 outbox 记录，重复写入应合并到同记录

## 8. Tools：内置工具契约（供 `Action.type=Invoke` 使用）

### 8.1 `memory_search`

Args：

```json
{
  "query": "上周讨论的架构",
  "scope": "private|public|auto",
  "top_k": 6
}
```

`scope` 规则：

- `private`：仅检索 private 记忆
- `public`：仅检索 public 记忆
- `auto`：
  - `channel_type=dm`：等价于 `private+public`
  - `channel_type=group|channel`：等价于 `public`
  - 若缺少 channel 上下文，默认退化为 `public`

Result：

```json
{
  "disabled": false,
  "hits": [
    {
      "path": "memory/2026-03-02.md",
      "scope": "private",
      "lines": [120, 145],
      "score": 0.62,
      "preview": "..."
    }
  ]
}
```

### 8.2 `memory_get`

Args：

```json
{
  "path": "memory/2026-03-02.md",
  "lines": [120, 145]
}
```

Result：

```json
{
  "path": "memory/2026-03-02.md",
  "content": "120: ...\n121: ...\n"
}
```

安全要求：

- `path` 必须位于 workspace 白名单目录
- 行范围与字节数有上限（例如 `max_chars=8000`）

### 8.3 Tools 统一错误格式与 `disabled` 语义

工具失败应返回统一结构：

```json
{
  "error": {
    "code": "FORBIDDEN",
    "message": "scope denied",
    "details": {
      "required_scope": "memory:read"
    }
  }
}
```

约定：

- `disabled=true`：功能被全局开关关闭（非权限问题）
- `error.code=FORBIDDEN`：功能开启但当前请求权限不足

### 8.4 `skill_apply_patch` / `workflow_apply_patch`（P1）

默认 high risk，必须审批。

一致性：

- `expected_base_hash` 不匹配返回 `CONFLICT`
- `patch_idempotency_key` 重复返回首次结果
- `expected_base_hash` 计算规则：`sha256(raw_bytes_of_target_file)`（按原始 bytes，不做解码，不处理 BOM/换行归一）
- 目标文件不存在时返回 `NOT_FOUND`

## 9. Skills Runner 协议（Process / HTTP）

### 9.1 Process Runner（stdin/stdout JSON）

输入（stdin）：

```json
{
  "request_id": "req_01H...",
  "skill": {
    "name": "calc",
    "version": "1.0.0"
  },
  "context": {
    "tenant_id": "local",
    "conversation_id": "conv_001",
    "session_key": "sk:9f5a3b7c...",
    "session_id": "s_20260303_001",
    "workspace_root": "/path/to/workspace",
    "scopes": ["..."],
    "timeout_ms": 30000
  },
  "args": {
    "expression": "(1+2)*3"
  }
}
```

输出（stdout）成功：

```json
{
  "request_id": "req_01H...",
  "ok": true,
  "result": {
    "value": 9
  },
  "logs": ["...optional..."]
}
```

输出（stdout）失败：

```json
{
  "request_id": "req_01H...",
  "ok": false,
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "bad expression"
  }
}
```

运行约束：

- `timeout_ms` 超时由 runner kill
- stdout 必须为合法 JSON
- stderr 必须捕获并写入 RunTrace（建议截断到固定长度）
- 进程 exit code 非 0 视为 `INTERNAL`，`error.message` 包含截断 stderr
- runner 工作目录固定为 `workspace_root`
- 环境变量采用白名单透传，默认不继承敏感 env
- 建议限制 `max_stdout_bytes`、`max_stderr_bytes`、`max_memory_mb`

### 9.2 HTTP Runner（可选）

- Endpoint：`POST http://localhost:<port>/invoke`
- 请求/响应结构与 Process Runner 一致

## 10. 状态机与一致性要求

### 10.1 Event 处理状态（逻辑）

- 执行状态：`accepted -> running -> committed|failed`
- 投递状态：`not_applicable|pending|sent|suppressed|failed`
- `sent` 仅表示真实发送成功，不等价于“已写 spool”
- `suppressed` 用于 `run_mode=NO_REPLY`

### 10.2 Approval 状态机

- `pending -> approved|rejected|expired`
- `approve/reject` 操作必须幂等

### 10.3 Outbound 状态机（spool 为 P0）

- `pending -> sent|failed`
- `failed -> pending`（`retry_count++`）

### 10.4 Run Commit 规则

- 一个 run 的 entries 必须 `AppendBatch` 成组写入
- 每组最后必须有 `commit marker`
- `entry_count` 不包含 `commit` 自身
- `last_entry_id` 必须指向该 batch 最后一个业务 entry
- 尾部缺 marker 视为未提交，恢复时丢弃
- 回滚语义：仅允许“丢弃尾部未提交 entries”，不允许在线修复或重写历史 entries
- `runs/<run_id>.json` 写入失败时，`sessions.json` 不得前移指针

### 10.5 崩溃恢复规则

1. 优先读取 `runtime/sessions.json`
2. 若不可读/为空，扫描 `runtime/sessions/*.jsonl` 重建最小索引
3. 重建优先用 header 中 `session_key`，缺失则挂 `unknown:<session_id>`
4. 使用 `latest compaction summary + tail entries` 重建上下文窗口

## 11. 版本化与兼容性

- HTTP API 统一 `/v1/`
- JSONL header 必须有 `format_version`
- Tool/Skill schema 通过 `version` 字段演进

## 12. 附录：Canonical JSON 与 Hash 规范

### 12.1 `payload_hash` 计算规则

冲突检测场景：同 `idempotency_key` 二次请求。

定义：

- `payload_hash = sha256(canonical_json(normalized_request))`
- 序列化格式：`sha256:<hex-lowercase>`

`normalized_request`：

- 保留：`source`, `conversation`, `payload`, `idempotency_key`
- 忽略：`timestamp`, `tenant_id`, `scopes`, `session_key`, `event_id`, `received_at`（服务端注入或易漂移字段）

`canonical_json` 规则：

- UTF-8 编码
- 对象键按字典序排序
- 无多余空白
- 数组保持原顺序
- null 字段可保留，不做自动删除
- 数值序列化遵循 Go `encoding/json` 默认行为（浮点按最短可逆表示），不得做二次格式化
- Unicode 字符按 UTF-8 原字符参与哈希；若编码器输出 `\uXXXX`，需保证语义等价且稳定
- `attachments` 作为数组按输入顺序参与哈希；上传顺序不同视为不同 payload（避免隐式重排）

### 12.2 JSONL entry 最小字段

- `header`：`type, session_id, session_key, created_at, format_version`
- 普通 entry：`type, entry_id, run_id`
- `commit`：`type=commit, entry_id, run_id, commit_id, entry_count, last_entry_id`

### 12.3 commit marker 校验

恢复时校验：

1. 从尾部向前找最近 `type=commit`
2. 从该 marker 向前数 `entry_count` 条业务 entry，最后一条 `entry_id` 必须等于 `last_entry_id`
3. 上述 `entry_count` 条业务 entry 必须 `run_id` 一致且等于 marker 的 `run_id`
4. marker 之后若仍有 entry，视为未完成尾部并丢弃
5. `checksum` 可选；启用时必须匹配，否则回滚到前一 marker
6. “回滚到前一 marker”定义为：丢弃该前一 marker 之后的所有尾部 entries，不做原地修复

## 13. 与架构文档对齐检查

- Gateway 入站幂等 + ResolveSession：已对齐
- `session_key/session_id` 双层语义：已对齐
- Event 状态查询（`GET /v1/events/{event_id}`）：已补齐
- AppendBatch + commit marker + last_entry_id：已对齐
- StoreLoop ack 后出站：已对齐
- NO_REPLY 静默回合边界：已对齐
- action 级副作用幂等：已对齐
- 健康检查端点（`GET /healthz`、`GET /readyz`）：已对齐
- 审批详情查询（`GET /v1/approvals/{id}`）：已对齐
- 背压与队列策略（`eventCh` 有界队列 + `503 QUEUE_FULL`）：已对齐
- DLQ 回补策略（DLQ Retry Worker + 指数退避）：已对齐
- 限流契约（token bucket + `429 RATE_LIMITED`）：已对齐
