# 需求分析文档（PRD）v0.4

## 1. 背景与动机

目标是实现一个“简化版 Go 实现的 OpenClaw 风格 Agent Runtime”，核心差异化聚焦在以下四点：

- 长期记忆自我进化：以天为单位沉淀 Markdown 记忆，并支持 RAG 检索。
- 系统技能稳定：System Skills 固化，只读，不允许 agent 修改。
- 用户技能/工作流可演化：User Skills / Workflows 允许提案式修改，并产出每日变更总结，支持回滚。
- 上下文管理不是硬截断：通过 flush、摘要、RAG 召回实现“上下文可丢弃但能力增强”。

工程目标是可在简历中清晰表达的系统能力：幂等、会话串行（lane/session）、HIL（Human-in-the-loop）、可观测、可回放。

## 2. 产品定位与目标

### 2.1 产品定位

本地优先的 Agent Runtime 内核，支持：

- 多渠道事件接入
- 会话（lane/session_key）串行执行
- 工具/技能调用
- 分层记忆与 RAG
- 用户技能/工作流自我进化
- 审批与风控
- 全链路 trace 与回放

### 2.2 Goals

1. 正确性优先：同一会话（lane/session_key）内严格串行；入站/出站均幂等。
2. 可解释与可回放：任意回复/行动都可追溯其记忆检索、工具调用、patch 变更。
3. 自我进化闭环：可提出 User Skills / Workflows 改进建议，并生成每日变更摘要，支持回滚。
4. 记忆与上下文管理：历史可压缩、可丢弃，但通过 flush -> md -> 索引 -> RAG 召回保证重要信息可恢复。

### 2.3 Non-goals（v1 不做或弱化）

- 多平台 IM 全覆盖（v1 仅 CLI + Telegram Bot 渠道）
- 语音、浏览器自动化、复杂 UI
- 大规模分布式（优先单机，预留扩展点）
- 企业级 RBAC 全量能力（v1 固定单租户 `tenant_id=local`，仅最小 scopes）

## 3. 术语与核心概念

### 3.1 InternalEvent（内部事件）

统一事件格式，来源包括：外部渠道消息（CLI/Telegram）、按钮回调、附件事件、cron 调度、审批结果。

### 3.2 Session（会话执行通道，Lane 同义）

`session_key` 计算规范（`SK-FORMULA-001`，本文件为唯一规范源）：

`session_key = "sk:" + sha256(tenant_id + "|" + conversation_id + "|" + channel_type + "|" + participant_id_or_dash + "|" + dm_scope)`

执行语义：

- 基线要求：同 `session_key` 禁止并发状态写入。
- v0.4 实现取舍：单 `EventLoop` 全局串行推进，作为“同 session 串行”的超集简化实现；并行仅用于单事件内慢调用隔离（RunnerPool）。
- 演进方向：后续可扩展为“分片或 per-session 串行执行”，前提是不破坏上述基线要求。

补充约定：

- `participant_id_or_dash`：`channel_type=dm` 时取 `participant_id`，否则固定为 `"-"`。
- `dm_scope`：v0.4 固定 `default`，字段保留用于未来区分同一会话内不同上下文域（如 `work/personal`）。
- `thread_id` 不参与 `session_key` 计算，仅用于展示与 trace 辅助信息；因此同一 `conversation_id` 下不同 `thread_id` 在 v0.4 共享同一会话上下文。

说明：v1 为单租户本地部署，`tenant_id` 固定为 `local`（字段保留用于后续扩展）。

### 3.3 ActionPlan（行动计划）

Agent Engine 输出结构化 Action 集合，不直接执行；由 Policy/HIL 决策是否执行。

### 3.4 Skills 分层

- System Skills（只读）：内置随版本发布，不可被 agent 修改。
- User Skills（可演化）：位于 workspace，可由 Evolution Engine 生成 patch 并受控应用。

### 3.5 Memory 分层

- Session 短期记忆：对话窗口
- Daily MD：`memory/YYYY-MM-DD.md`（追加）
- Curated：`MEMORY.md`（稳定事实与偏好）
- KB（可选）：`kb/**/*.md`

## 4. 用户画像与使用场景

### 4.1 用户画像

- 主要用户：个人开发者/知识工作者
- 次要用户：小范围群聊协作（需 public/private 记忆隔离）

### 4.2 典型场景（User Stories）

1. 在 Telegram/CLI 发一句话，系统可连续调用工具并组织回复。
2. 用户说“记住我更喜欢 Go”，系统写入当日记忆，并在后续行为体现偏好。
3. 用户问“上周讨论方案”，系统先 `memory_search` 再引用片段回答。
4. 系统对某任务常拆错，能在每日反思中优化 user skill/workflow，提高后续稳定性。
5. 高风险动作（写文件/改技能/外部请求）进入审批卡片，确认后执行。

## 5. 功能需求（Functional Requirements）

优先级定义：

- P0：MVP 必须
- P1：强烈建议
- P2：可选增强

### 5.1 渠道接入与事件归一化（Spokes / Adapters）

- FR-CH-001（P0）：支持至少两种入口：CLI + Telegram Bot（long polling）。
- FR-CH-002（P0）：入站鉴权/限流（HTTP 入口支持 Header API Key；Telegram 使用 bot token + user/chat allowlist；本地 CLI 支持本地信任模式；单租户粒度限流）。
  - 限流算法：token bucket（可配置）。
  - 默认阈值：`tenant_id` 维度 `30 req/s, burst=60`；`session_key` 维度 `5 req/s, burst=10`。
  - 超限行为：返回 `429 RATE_LIMITED`，并附 `Retry-After`（秒）。
- FR-CH-003（P0）：归一化为 InternalEvent，至少覆盖 Telegram 文本消息、按钮回调（callback_query）与附件元信息（document/photo/video 的基础字段）。
- FR-CH-004（P0）：调用方/适配器必须提供稳定 `idempotency_key`；同一上游事件重试必须复用该 key。Ingress 对缺失 key 的请求返回参数错误，不使用接收时刻兜底生成。v1 规范：Telegram 使用 `telegram:update:<update_id>`，CLI 使用 `cli:<conversation_id>:<seq>`。

### 5.2 Ingress Inbox 去重与路由

- FR-IN-001（P0）：Inbox 去重，`(tenant_id, idempotency_key)` 唯一约束；重复事件直接 ACK。
- FR-IN-002（P0）：路由到会话执行通道：按 `SK-FORMULA-001` 计算 `session_key`，投递至 EventLoop 主处理队列。
- FR-IN-003（P0）：主处理队列必须有界并具备背压语义；队列满或入队超时时不得静默丢弃，必须返回可重试错误（`QUEUE_FULL` + `Retry-After`）并记录指标。

### 5.3 EventLoop：会话运行时（Conversation Runtime）

- FR-LA-001（P0）：EventLoop 默认全局串行消费 `InternalEvent`；该策略作为“同一 `session_key` 不并发写入”的超集实现（见 `3.2`）。
- FR-LA-002（P0）：会话状态管理基于 `runtime/sessions.json + runtime/sessions/<session_id>.jsonl`（事实源），不依赖 snapshot/CAS 作为一致性机制。
- FR-LA-003（P0）：每个事件触发一次 Run（可含多轮 tool calls），生成 `run_id` 与 RunTrace，并按 run 级提交落盘。
- FR-LA-004（P1）：支持重试/死信/延迟（最大重试次数、DLQ、延迟任务）。

### 5.4 Agent Engine：Interpret -> Plan -> Act -> Observe -> Verify -> Respond

- FR-AE-001（P0）：Context Assembler 注入 bootstrap（SOUL/USER/AGENTS/TOOLS）、skills 摘要、history window、RAG snippets，并执行预算分配与剪枝。
- FR-AE-002（P0）：支持多轮 tool calling，设 `max_tool_rounds` 防死循环。
- FR-AE-003（P0）：输出 ActionPlan（不是直接执行），至少含 SendMessage、CallTool/CallSkill、WriteMemory、RequestApproval。
- FR-AE-004（P1）：Verify/补偿：工具失败重试或降级、结构化输出 schema 校验与一次模型修正回合。

### 5.5 Tool / Skills 系统（System vs User）

- FR-SK-001（P0）：Tool Registry，统一 tool spec：name、desc、args schema、permissions、risk_level。
- FR-SK-002（P0）：Skill Loader 分层：System Skills（只读目录）+ User Skills（`workspace/skills`，可开关）。
- FR-SK-003（P0）：Skill Runner（MVP 二选一）：Process Runner（stdin JSON -> stdout JSON）或 HTTP Runner（POST JSON -> JSON）。
- FR-SK-004（P0）：权限声明与校验（如 `fs_read`/`fs_write`/`net_http`/`skill_patch`），默认 deny，显式 allow。
- FR-SK-005（P1）：命令分发（可选）：`/xxx` 触发指定 skill/workflow。

### 5.6 Memory & RAG（分层记忆 + 检索工具）

- FR-MEM-001（P0）：Daily memory 追加写入 `workspace/memory/YYYY-MM-DD.md`，带时间戳与来源。
- FR-MEM-002（P0）：Curated memory 更新到 `workspace/MEMORY.md`（v1 可先由 `/curate_memory` workflow 手动触发）。
- FR-RAG-001（P0）：`memory_search` 工具，输入 `query/scope/topK`，输出 snippets（path、line_start、line_end、score、preview）。
- FR-RAG-002（P0）：`memory_get` 工具，输入 path + line range，返回精确片段；强制路径白名单，禁止越界读。
- FR-RAG-003（P1）：Indexer 监听 `memory/**/*.md`、`MEMORY.md`（可扩展 `kb/**/*.md`）；MVP 用文件倒排索引（BM25），向量/hybrid 为 P1。
- FR-RAG-004（P1）：public/private 记忆隔离，群聊默认不检索 private。

### 5.7 Policy & Human-in-the-loop（审批门）

- FR-POL-001（P0）：风险分级（low/medium/high），依据 action 类型、tool 权限、channel、workspace 策略。
- FR-POL-002（P0）：高风险动作进入 PendingApproval；生成审批卡片/链接（v1 可 CLI/简易网页）；审批事件回流后续执行。
- FR-POL-003（P1）：策略可配置（workspace + user/chat allowlist/denylist，自动执行开关）。

### 5.8 Outbound Hub + Outbox（出站可靠投递）

- FR-OUT-001（P0）：Outbox 幂等，SendMessage 生成 outbound_idempotency_key，防止重试重复发送。
- FR-OUT-002（P0）：回执追踪（pending/sent/failed）+ 指数退避重试。
- FR-OUT-003（P0）：`runtime/outbound_spool` 与 Retry Worker 必须启用；adapter 直发失败后必须先持久化到 spool，再按重试策略投递，禁止静默丢失。

### 5.9 自我进化（Evolution Engine）

- FR-EVO-001（P1）：生成 user skill/workflow 改进提案（Propose）：输入 RunTrace + 失败统计 + 用户反馈（可缺省），输出 patch(diff) + 解释 + 风险评估。
- FR-EVO-002（P1）：每日变更总结：`workspace/evolution/YYYY-MM-DD.md`，包含改动列表、原因、影响面、回滚说明。
- FR-EVO-003（P1）：Apply/Rollback：Apply 受 Policy Gate 控制（默认审批或开关）；保留 patch 历史，支持回滚到上一版本。

说明：v1 可先做“提案 + 日报”，不自动 apply，以降低风险。

## 6. 非功能需求（NFR）

### 6.1 可靠性与一致性

- NFR-REL-001（P0）：入站/出站幂等保证（含 Outbox + spool 持久化重试语义）。
- NFR-REL-002（P0）：同会话串行一致性（禁止并发写会话状态；lane 为历史同义词）。
- NFR-REL-003（P1）：崩溃恢复（重启后可续处理未完成 outbox，及未 ACK 事件的重消费能力）。

### 6.2 性能与并发

- NFR-PERF-001（P0）：v0.4 单机默认 EventLoop 全局串行推进（正确性优先于吞吐）；并发优化仅用于单事件内慢调用隔离（RunnerPool）。
- NFR-PERF-002（P1）：每次 run 设上限：最大工具轮数、最大 token、最大执行时间。

### 6.3 安全与隐私

- NFR-SEC-001（P0）：工作区路径边界（如 `memory_get` 只能读取白名单）。
- NFR-SEC-002（P0）：敏感信息不落日志（API key、token）。
- NFR-SEC-003（P1）：工具/技能最小权限（deny by default）。
- NFR-SEC-004（P1）：public/private 记忆隔离默认安全。

### 6.4 可观测与可调试

- NFR-OBS-001（P0）：结构化日志（event_id、run_id、session_key）。
- NFR-OBS-002（P0）：RunTrace 可查询（CLI/HTTP）。
- NFR-OBS-003（P1）：基础指标（latency、tool error rate、token 估算）。

### 6.5 可移植与部署

- NFR-DEP-001（P0）：单机可运行（单二进制 + workspace）。
- NFR-DEP-002（P1）：配置文件 + 环境变量覆盖，便于 CI/CD。

## 7. 约束与假设

- v1 默认单机运行，队列可先用“内存队列 + 文件账本持久化 inbox/outbox”。
- LLM provider 先支持 1~2 家或一个 OpenAI-like endpoint，不以多供应商覆盖为目标。
- skills/workflows 先采用“文件 + schema”格式，不追求复杂插件生态。
- v1 固定单租户：`tenant_id=local`；鉴权覆盖 HTTP Header API Key 与 Telegram allowlist，本地 CLI 允许本地信任模式。

## 8. 风险与关键权衡

1. 自动修改技能风险：必须配套审批、回滚、回归测试护栏。
2. RAG 质量风险：MVP 用 BM25 先闭环，向量/hybrid 后置到 P1。
3. 多渠道复杂性风险：v1 只做 CLI + Telegram，架构预留扩展。
4. 工具调用稳定性风险：需要 schema 校验 + `max_tool_rounds`。

## 9. MVP 验收标准（可量化）

- 同一 `conversation_id` 的消息严格顺序处理（可通过日志/trace 验证）。
- 重复 Telegram update 重放不会导致重复回复（Inbox + Outbox 幂等生效）。
- `memory_search` 返回包含路径与行号的 snippets；`memory_get` 返回精确片段且不可越界。
- Agent 至少完成 2 个工具回合（tool_call -> result -> final）。
- 高风险 action 进入 PendingApproval，并可在 approval event 后继续执行。
- 生成 evolution 日报文件（即使“本日无改动/提案”）。
