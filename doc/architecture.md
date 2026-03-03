# 系统架构文档 v0.4

> 本文基于 [requirements.md](./requirements.md)，描述当前版本的事件驱动会话架构：`Gateway + sessions.json + <session_id>.jsonl`。  
> 目标：在单机单进程场景下，以正确性、可回放、可恢复为优先。

## 1. 架构目标与原则

### 1.1 核心目标

1. 同会话强一致：同一 `session_key` 严格串行，避免并发写导致状态错乱。
2. 无数据库持久化：核心状态全部落文件系统，不依赖任何数据库或关系型事务。
3. 可解释与可回放：每次运行产出 RunTrace，可复盘上下文装配、工具调用、审批与出站。
4. 安全边界清晰：Workspace 边界、最小权限、public/private 记忆隔离、高风险动作走 HIL。
5. 支持自我进化闭环：Evolution Engine 提案修改 User Skills/Workflows，输出每日变更总结并支持回滚。

定位声明（v0.4）：

- 单机单 Gateway、单写者存储、面向单人多会话。
- 并发优化不是首要目标，正确性与可恢复优先。

### 1.2 设计原则

- Hub-and-Spoke：多渠道统一归一为 `InternalEvent`。
- 两层会话标识：
  - `session_key`：路由键，计算规则引用 [requirements.md §3.2](./requirements.md) 的 `SK-FORMULA-001`（本文件不重复定义公式）。
    - `participant_id_or_dash` 规则：`channel_type=dm` 时取 `participant_id`，否则固定为 `"-"`。
    - `thread_id` 不参与 `session_key` 计算，仅用于展示与 trace 辅助信息；同一 `conversation_id` 下不同 thread 在 v0.4 共享同一会话上下文。
  - `session_id`：实际持久化转写文件标识（可 reset/idle/daily 轮换）。
- 事实源优先：`runtime/sessions/<session_id>.jsonl` 是事实源；`sessions.json` 是索引/指针。
- Plan/Execute 分离：Agent 只产出 `ActionPlan`，Policy/HIL 决策后再执行。
- 单写者落盘：所有状态写入只允许通过 `StoreLoop` 串行执行。
- 后台改写事件化：会改会话状态的后台任务必须变成 `InternalEvent` 走主链路。

### 1.3 关键约束（无 DB 版本）

- 不使用数据库事务；一致性由 `EventLoop + StoreLoop + AppendBatch` 保证。
- Gateway 必须执行 `ResolveSession`：`session_key -> sessions.json -> active_session_id`（即当前 `session_id`）。
- 一次 AgentRun 的 transcript 必须 run 级提交，不允许半截写入。
- 出站默认直连 adapter；失败必须写 `outbound_spool` 后由 Retry Worker 异步重试（P0）。

## 2. 高层组件图

```text
[Channels: CLI / Telegram / (Optional WebUI)]
        |
        v
[Spokes: Channel Adapters]
  - 验签/鉴权/限流（HTTP: `Authorization: Bearer <API_KEY>`；Telegram: bot token + `chat_id/user_id` allowlist）
  - 原生消息解析 -> InternalEvent
  - 生成 idempotency_key
        |
        v
[Gateway]
  - tenant & scopes 绑定
  - 入站幂等检查（in-memory LRU + file ledger）
  - ResolveSession(session_key -> active_session_id=session_id)
        |
        v
[EventLoop]
  - 串行处理 InternalEvent
  - 驱动 AgentRun / Policy / Dispatcher
        |
        v
[StoreLoop (Single Writer)]
  - AppendBatch -> runtime/sessions/<session_id>.jsonl
  - 写 runs/<run_id>.json
  - 原子更新 sessions.json（tmp+rename）
        |
        v
[Outbound Adapter]
  - 发送到外部平台
  - 失败写 outbound_spool（必选）
```

后台能力（分两类）：

- 只读型：Indexer（可旁路读取，不写会话状态）。
- 改写型：Compaction / memory flush / approvals / cron 触发，必须转成 `InternalEvent`。

## 3. 核心数据流与时序

### 3.1 入站消息处理时序（正常链路）

```text
Channel -> Adapter -> Gateway(dedupe + ResolveSession) -> EventLoop
  -> AgentRun:
       Context Assemble (bootstrap + skills + RAG + history)
       LLM call (may return tool_calls)
       produce ActionPlan
  -> Policy Gate:
       low -> execute actions
       high -> create PendingApproval + outbound approval message
  -> StoreLoop Commit:
       append transcript (AppendBatch)
       write run trace
       update sessions.json (tmp+rename)
  -> wait StoreLoop ack
  -> outbound adapter send
```

### 3.2 一致性边界（替代“单事务”）

Run 级提交顺序固定为：

1. `append transcript (AppendBatch)`
2. `write run trace`
3. `update sessions.json (tmp+rename)`

一致性机制：

- 单线程 EventLoop 负责状态推进。
- 单写者 StoreLoop 负责所有落盘。
- `sessions/<session_id>.jsonl` 是事实源，`sessions.json` 仅是指针索引。
- 崩溃恢复不依赖 `snapshot_version`，而是从 `sessions.json` 找 `active_session_id`，再读取 `jsonl` 的 `latest compaction summary + tail entries` 重建上下文。
- `runs/<run_id>.json` 写入失败时，本次 commit 视为失败，不更新 `sessions.json`。
- 只有 StoreLoop 返回 commit 成功 ack 后，Outbound 才允许发送，避免“消息已发但未落盘”。

### 3.3 工具调用一致性（Run Commit / AppendBatch）

P0 规则：

- 一次 AgentRun 产生的 transcript entries 必须批量提交：
  - `entries := [user_message, assistant(tool_calls), tool_results..., assistant(final)]`
  - `AppendBatch(entries + commit_marker)` 一次落盘，再更新 `sessions.json`。
- `tool_result` 必须引用 `tool_call_id`，确保成对可追踪。
- `compaction` / `memory_flush` 事件不得插入未提交 run 的中间。
- `commit_marker` 必须包含 `commit_id/run_id/entry_count/checksum(optional)`。
- 重启恢复时若发现尾部 batch 缺失 `commit_marker`，必须丢弃未完成尾部 entries（回滚到上一个已提交 marker）。

### 3.4 审批（HIL）闭环

```text
AgentRun -> ActionPlan(high risk) -> Policy Gate -> PendingApproval(file)
  -> Outbound send approval card/link
User approves -> Approval Endpoint -> InternalEvent(ApprovalGranted)
  -> Gateway -> EventLoop -> continue pending actions
```

### 3.5 Silent/Internal Run（NO_REPLY）

适用场景：

- memory flush
- compaction
- 索引刷新触发（仅需要会话内记录时）

行为约束：

- `run_mode = NO_REPLY` 时，Outbound 默认抑制，不发送用户消息。
- 默认仅允许写入 `workspace/memory/**` 与 `runtime/**` 系统目录；其它目录写入、外网请求、skill/workflow patch 均视为高风险副作用并默认禁止。
- `run_mode = NO_REPLY` 默认不产生任何用户可见 side effect（如 typing indicator、草稿消息、临时状态）。
- 系统内务写入（`runtime/sessions/*.jsonl`、`runtime/runs/*`、`runtime/idempotency/*`、`runtime/outbound_spool/*`、`runtime/dead_letters/*`）不属于高风险副作用，允许在 NO_REPLY 下执行。
- 仍需写 RunTrace，便于审计与回放。

## 4. 并发模型：EventLoop + StoreLoop（简化版）

### 4.1 会话标识与轮换

- `session_key` 计算规则引用 [requirements.md §3.2](./requirements.md) `SK-FORMULA-001`。
- `active_session_id` 是当前活跃会话文件标识（即当前 `session_id`）。
- 可触发 `session_id` 轮换的场景：
  - 用户显式 reset。
  - idle 超过阈值。
  - 按日切换（可选）。

`ResolveSession` 伪流程：

1. 若 `sessions.json` 无该 `session_key`：创建新 `session_id`，写入 jsonl header，更新 `sessions.json`。
2. 若命中轮换条件：创建新 `session_id`，在旧 `session_id`.jsonl 追加 `type=session_rotate`（记录 from/to/reason），再更新 `sessions.json` 指针。
3. 若无需轮换：返回 `active_session_id`。

### 4.2 运行循环职责

- EventLoop：
  - v0.4 采用方案 A：一次只处理一个 `InternalEvent`（全局串行）。
  - 当前事件未收到 Runner/StoreLoop 结果前，不处理下一事件。
  - 负责推进状态机、触发 AgentRun、生成写入请求。
  - 该策略是“同 `session_key` 串行”的超集实现；后续若演进为 per-session 并行，必须保持同 `session_key` 不并发写。
- StoreLoop（单写者）：
  - 串行处理所有写入（`runtime/sessions.json`、`runtime/sessions/*.jsonl`、`runtime/runs/`、`runtime/idempotency/*.jsonl`、`runtime/cron/jobs.json`）。
  - 对索引文件统一使用 `tmp + fsync + rename`。
- RunnerPool（可选，1-2 worker）：
  - 执行慢调用（LLM/tool），降低 EventLoop 阻塞时间。
  - 任何写盘结果必须回到 StoreLoop 提交。

### 4.3 Go 实现参考（informative，非规范）

- 通道拆分：
  - `eventCh`：Gateway -> EventLoop
  - `storeReqCh`：EventLoop/Runner -> StoreLoop
  - `storeAckCh`：StoreLoop -> EventLoop
- 不引入 per-session worker 生命周期管理（lane 为历史同义词）。
- 并发目标是隔离慢 I/O，不是追求高吞吐。
- EventLoop 严格串行推进；RunnerPool 只用于当前事件内的慢调用隔离。

### 4.4 背压与队列策略（P0）

- `eventCh` 必须是有界队列（默认 `capacity=1024`，可配置）。
- 队列满时 Gateway 不得静默丢弃事件，必须立即返回错误：
  - 外部 HTTP ingest：`503 SERVICE_UNAVAILABLE`，`error.code=QUEUE_FULL`，并返回 `Retry-After`（秒）。
  - 内部生产者（cron/approval）：写入 `dead_letters/events.jsonl` 并按 DLQ 回补策略重试。
- Gateway 入站处理禁止无限阻塞；超过 `ingest_enqueue_timeout`（默认 `200ms`）按队列满处理。
- 背压命中必须打点与日志：`event_queue_depth`、`event_queue_reject_total`。

DLQ 回补策略（P0）：

- 执行者：独立 `DLQ Retry Worker`（不由 EventLoop 主循环直接轮询文件）。
- 重试算法：指数退避 + 抖动（默认 `base=1s`、`factor=2`、`max_interval=5m`）。
- 最大重试次数：默认 `8` 次；超限后标记终态 `failed_permanent` 并保留在 DLQ。
- 调度依据：仅当 `retryable=true` 且 `now >= next_attempt_at` 才可回补。
- 优先级：正常新入站事件优先，DLQ 回补为低优先级（防止恢复流量饿死实时流量）。
- 回补写回：每次失败必须更新 `retry_count/last_error/next_attempt_at`，成功后写 `recovered_at` 并归档。

## 5. 逻辑分层与模块边界

### 5.1 Interface Layer（Spokes）

Channel Adapters：

- 输入：外部事件（Telegram polling/CLI）。
- 输出：`InternalEvent`。
- 关注点：鉴权/限流、原生消息解析、`idempotency_key` 生成。
- 幂等键规范：Telegram 用 `telegram:update:<update_id>`；CLI 用 `cli:<conversation_id>:<seq>`。

### 5.2 Control Plane（Gateway/Resolver/EventLoop/StoreLoop）

Gateway：

- 接收 InternalEvent（adapter/approval/cron）。
- 绑定 `tenant_id=local` 与 `scopes`。
- 入站幂等检查（内存 LRU + 文件账本）。
- 若事件落盘失败或入队失败，必须回滚本次入站幂等登记，保证同一 `idempotency_key` 可重试。
- ResolveSession：从 `sessions.json` 获取 `active_session_id`。
- 健康检查端点：
  - `GET /healthz`：进程存活探针。
  - `GET /readyz`：就绪探针（StoreLoop、workspace 可写、关键 worker 就绪）。

Idempotency Ledger（文件）：

- 入站去重：`runtime/idempotency/inbound_keys.jsonl`。
- 出站去重：`runtime/idempotency/outbound_keys.jsonl`。
- 副作用幂等：`runtime/idempotency/action_keys.jsonl`（`action_idempotency_key=run_id:action_index`）。

Cron 管理：

- 配置来源：`runtime/cron/jobs.json`。
- 到点只做一件事：发 `InternalEvent(cron_fire, session_key=...)`。
- 禁止 cron worker 直接写会话文件。
- `jobs.json` 可能在运行时写回，运维上建议停机修改后再启动。

### 5.3 Execution Plane（Runtime + Agent + Dispatcher）

Conversation Runtime：

- 驱动一次事件的完整执行。
- 管理 pending 状态（审批/延迟任务）。

Agent Engine：

- Context Assembler：上下文组装、预算分配与剪枝。
- LLM Hub：provider 适配、超时、重试。
- Tool/Skill Spec：schema 与权限约束注入。
- 输出：`ActionPlan`。

Policy & HIL Gate：

- 输入：ActionPlan + scopes + channel_type + workspace policy。
- 输出：可执行 actions 或 PendingApproval。

Action Dispatcher：

- 执行动作：Tools/Skills、Memory Writer、Evolution（提案/应用/回滚）、Outbound 调用。
- 每个 action 执行前先检查 `action_idempotency_key`，命中则跳过副作用执行。
- 失败动作生成补偿事件（重试/降级/进入 dead letter）。

### 5.4 Outbound Plane（Adapter + Retry Worker）

Outbound Adapter：

- 将内部消息格式转为平台格式并调用 `adapter.send`。
- 使用 `outbound_idempotency_key` 防重复发送。

Retry Worker（必选）：

- 消费 `runtime/outbound_spool/*.jsonl`。
- 处理指数退避与最大重试次数。

## 6. 上下文装配（Context Assembler）

### 6.1 输入项

- bootstrap（SOUL/USER/AGENTS/TOOLS）
- skills（system + enabled user）
- 会话历史：`latest compaction summary + tail entries`
  - `tail entries` 定义：从最后一个 `compaction` entry 之后开始取，最多 `max_tail_entries` 条。
  - `compaction` entry 至少包含 `cutoff_commit_id` 或 `keep_from_entry_id`。
- 记忆检索结果（RAG snippets）
- 运行约束（policy/scopes/budget）
- pending 任务（可选）

### 6.2 预算策略（v0.4）

- Bootstrap：约 15%
- Skills：约 15%
- RAG：约 20%
- History：剩余弹性
- Safety/Policy：固定最小保留

剪枝优先级：

1. 保留 policy/safety 约束。
2. 保留工具 schema（保障 tool calling）。
3. 保留 RAG snippets（事实优先）。
4. 最后裁剪 history。

### 6.3 可解释性输出

Assembler 必须产出 `ContextManifest`（写入 run trace）：

- 注入文件（路径、截断标记）
- 选用 skills（名称、版本）
- RAG 命中（path、line range、score）
- history 覆盖范围（含 compaction 起点、`cutoff_commit_id` 或 `keep_from_entry_id`）

## 7. Memory/RAG 架构

### 7.1 Memory Writer（写入管道）

- daily：`workspace/memory/YYYY-MM-DD.md` 追加写入。
- curated：`workspace/MEMORY.md`（每日整理或手动触发）。

严格流程（事件化）：

1. token/长度逼近阈值
2. `InternalEvent(memory_flush, run_mode=NO_REPLY)`
3. `InternalEvent(compaction, run_mode=NO_REPLY)`

### 7.2 Indexer（索引管道）

监听并索引：

- `memory/**/*.md`
- `MEMORY.md`
- `kb/**/*.md`（P1）
- `skills/**/SKILL.md`（可选）

约束：

- Indexer 默认只读，不可直接修改会话状态文件。
- 需要改写会话时，必须发事件给 Gateway。

### 7.3 标准检索工具

- `memory_search`：返回 snippets（path + line range + score + preview）。
- `memory_get`：精确读取片段，必须经过 Workspace FS Guard。

## 8. Skills 体系：System vs User + Evolution

### 8.1 目录与加载优先级

- System Skills：`$APP_HOME/system_skills/`（只读，随版本发布）。
- User Skills：`workspace/skills/<name>/`（可变，可禁用）。

### 8.2 Skill 结构建议

- `skill.yaml`：name/version/permissions/entry/schema/risk_level
- `SKILL.md`：模型可读说明（调用方式、示例、约束）

### 8.3 Evolution Engine

输入：

- run trace
- 失败统计
- 用户反馈（可选）

输出：

- user skill/workflow 的 patch 提案（diff + 风险说明）
- 每日总结：`workspace/evolution/YYYY-MM-DD.md`

应用策略：

- 默认“提案 + 记录”，自动 apply 受 policy 控制。
- apply 前进行 schema/lint/回归校验（可分阶段启用）。

回滚：

- 记录 patch 版本与备份文件。
- 支持回滚至上一版本。

## 9. 存储架构（Workspace + File State Store）

### 9.1 Workspace 结构

```text
workspace/
  bootstrap/
    SOUL.md
    USER.md
    AGENTS.md
    TOOLS.md
  skills/
    <skill>/
      SKILL.md
      skill.yaml
  workflows/
    <wf>.yaml
  memory/
    YYYY-MM-DD.md
  MEMORY.md
  kb/                   # P1
  evolution/
    YYYY-MM-DD.md
  runtime/
    sessions.json                 # session_key -> active_session_id + stats
    sessions/
      <session_id>.jsonl          # append-only transcript（事实源）
    runs/
      <run_id>.json               # RunTrace
    approvals/
      pending/
      done/
    idempotency/
      inbound_keys.jsonl
      outbound_keys.jsonl
    cron/
      jobs.json                   # cron definitions
    dead_letters/
      events.jsonl                # P0 全局 DLQ
    outbound_spool/
      pending.jsonl               # 必选（P0）
  attachments/                    # P2
```

### 9.2 文件写入约束（关键）

- 删除“`session.json` 原子快照中心”作为主路径（可选 cache，不参与一致性判定）。
- 强化 `sessions.json` 原子替换：`tmp + fsync + rename`。
- 每个 `runtime/sessions/<session_id>.jsonl` 第一条必须是 `type=header`，至少包含 `session_id/session_key/created_at/format_version`。
- 强化 Run 级 `AppendBatch`：
  - 一个 run 的 entries 必须成组写入。
  - 同组 entries 共享 `run_id/commit_id`。
  - 每组最后一条必须是 `type=commit` marker（含 `commit_id/run_id/entry_count/checksum(optional)`）。
  - 未完成 commit 不得更新 `sessions.json` 指针。
- `runs/<run_id>.json` 一次写入，禁止覆盖式更新。
- `idempotency/*.jsonl` append + 周期压缩，避免无限增长。
- 所有状态文件写入必须经 StoreLoop，禁止旁路直接写。

最小 JSONL 记录示例：

```json
{"type":"header","session_id":"s_20260303_001","session_key":"sk:9f5a3b7c...","created_at":"2026-03-03T10:00:00Z","format_version":"1"}
{"type":"user","entry_id":"e1","run_id":"r1","content":"你好"}
{"type":"assistant","entry_id":"e2","run_id":"r1","tool_calls":[{"tool_call_id":"tc1","name":"memory_search"}]}
{"type":"tool_result","entry_id":"e3","run_id":"r1","tool_call_id":"tc1","ok":true,"result":"..."}
{"type":"assistant","entry_id":"e4","run_id":"r1","content":"已完成"}
{"type":"commit","entry_id":"e5","run_id":"r1","commit_id":"c1","entry_count":4}
```

## 10. 安全模型与边界

### 10.1 Scopes 与权限

- v1 固定单租户：`tenant_id=local`（字段保留，便于未来扩展）。
- `scopes` 约束访问能力（最小 RBAC）。
- 工具权限示例：`fs_read`、`fs_write`、`net_http`、`skill_patch`、`workflow_patch`。

### 10.2 Workspace FS Guard（关键）

所有文件读写必须经过统一封装：

- 路径白名单（仅 workspace 子目录）。
- 禁止符号链接逃逸。
- 对 `memory_get`/附件读取设置范围与大小上限。

### 10.3 Public/Private 记忆隔离

- 写入时标注 scope。
- `memory_search` 必须传 scope，群聊默认仅 `public`。
- `MEMORY.md` 默认 private，除非显式声明公开。

### 10.4 高风险动作与 NO_REPLY 约束

- 高风险动作必须走 HIL：
  - 非安全目录的 `fs_write`
  - skill/workflow patch apply
  - 非 allowlist 域名的 `net_http`
- `run_mode=NO_REPLY` 默认禁止高风险副作用；如需放开必须显式 policy allow。
- `run_mode=NO_REPLY` 默认禁止一切用户可见副作用（消息、typing、草稿、状态提示），除非 policy 显式放行。
- `run_mode=NO_REPLY` 允许系统内务写入（session jsonl、run trace、idempotency ledger、spool、DLQ），这些写入不视为高风险副作用。

## 11. 失败处理与恢复策略

### 11.1 入站（Gateway）

- 重复事件：幂等命中后直接 ACK。
- 处理失败：写入 `runtime/dead_letters/events.jsonl`（DLQ 触发范围包括背压拒绝与 Gateway 内部处理失败，不含 AgentRun/Outbound 层失败——后者由各自的重试机制处理）。
- DLQ entry 必须包含：`event_id/session_key/session_id/run_id/error_code/retryable/retry_count/last_error/next_attempt_at/recovered_at`。
- DLQ 重试由 `DLQ Retry Worker` 负责，不占用 EventLoop 主处理循环。

### 11.2 AgentRun / Runtime

- 结构化失败：写 run trace + error_code。
- 工具失败：按策略重试、降级或转人工审批。
- 副作用执行必须做 action 级幂等校验（`action_idempotency_key=run_id:action_index`）。
- 单次 run 失败不破坏已提交 commit；未提交 commit 可安全丢弃重跑。

### 11.3 出站（Adapter + Spool）

- 发送失败：必须先写入 spool，再按指数退避重试，禁止静默丢失。
- 发送成功：写回回执（若渠道返回 message_id）并登记 outbound 幂等账本。

### 11.4 崩溃恢复（v0.4）

重启后流程：

1. 读取 `runtime/sessions.json`；若不可读/为空，扫描 `runtime/sessions/*.jsonl` 重建最小索引（优先使用 header 的 `session_key`，缺失时挂为 `unknown:<session_id>`）。
2. 对每个 `session_key` 找 `active_session_id`（即当前 `session_id`）。
3. 读取 `runtime/sessions/<session_id>.jsonl`，校验尾部是否以 `commit marker` 结束。
4. 若尾部存在未完成 batch（缺失 commit marker），丢弃未完成尾部 entries。
5. 用 `latest compaction summary + tail entries` 重建上下文窗口。
6. 扫描 `approvals/pending` 与 `outbound_spool` 恢复外部流程。

## 12. 可观测性（Logging / Trace / Metrics）

### 12.1 结构化日志

必须字段：

- event_id、run_id、session_key、session_id、tenant_id、conversation_id
- module（adapter/gateway/eventloop/storeloop/agent/dispatcher/outbound）
- run_mode（NORMAL/NO_REPLY）、commit_id
- latency_ms、status、error_code

### 12.2 RunTrace（核心资产）

- ContextManifest（注入来源与裁剪信息）
- RAG hits（path/line/score）
- Tool calls（name、args/result 摘要、耗时）
- ActionPlan（动作与风险分级）
- LLM 调用（provider/model/token 估算、耗时、重试）
- CommitSummary（entry_count、commit_id、session_id、是否 silent）

### 12.3 Metrics（P1）

- EventLoop 等待时长
- RunnerPool 忙碌度
- tool error rate
- memory_search 命中率（粗略）
- outbound 失败率

## 13. 部署与运行形态（v0.4）

### 13.1 单机单二进制

同一进程内 goroutines：

- HTTP server（Gateway + Approval）
- Telegram poller worker（Long Polling）
- EventLoop（主控制循环）
- StoreLoop（单写者落盘）
- RunnerPool（可选，1-2 workers）
- Outbound retry worker（必选）
- DLQ retry worker（必选）
- Indexer watcher worker（只读）
- Cron ticker（仅触发 InternalEvent，不直接写状态）

### 13.2 配置体系

- `app.yaml` + 环境变量覆盖。
- 配置项包括 workspace 路径。
- 配置项包括 LLM endpoint/key。
- 配置项包括 budgets（token/rounds/timeouts）。
- 配置项包括 policy（auto_execute、allowlist、scopes、no_reply_guard）。
- 配置项包括 Telegram（bot token、allowed_chat_ids、allowed_user_ids、poll timeout）。
- 配置项包括 runtime（idle_rotate_minutes、daily_rotate、max_tail_entries）。
- 配置项包括 runner（pool_size、llm_timeout、tool_timeout）。
- 配置项包括 cron（jobs_file=`runtime/cron/jobs.json`）。

## 14. 扩展点

1. Channels：新增 adapter（实现 InternalEvent 输入与 send 输出）。
2. LLM Providers：新增 provider 适配器，复用统一接口。
3. Tool/Skill Runner：从 process 扩展到 HTTP、容器 sandbox。
4. Memory Backend：从 file-index 扩展到 hybrid 或外部向量库。
5. Web 控制台（P2）：基于 runs、approvals、jobs、skills 状态构建 UI。

## 15. 与 PRD 映射检查

- Hub-and-Spoke：Adapters + Gateway。
- 入站/出站幂等：文件账本（inbound/outbound key ledger）。
- 会话一致性：`session_key` 串行推进 + `session_id` 可轮换。
- Agent loop + ActionPlan：保留。
- Policy/HIL 审批门：保留。
- Memory（daily/curated）+ RAG：保留。
- Evolution（日更总结 + patch/rollback）：保留。
