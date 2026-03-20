# Feature Specification: 日志系统升级与链路观测补齐

**Feature Branch**: `002-logging-upgrade` (`speckit` feature id: `002-logging-upgrade`)  
**Created**: 2026-03-20  
**Status**: Draft  
**Input**: User description: "/speckit.specify 我想 对日志系统进行更新，目前日志种类很少，并且日志都是 json 形式的，我想使用正常的日志形式，并且再各个链路上多暴露些日志"

## Clarified Decisions

- 本次 scope 只覆盖进程运行日志，也就是服务端与后台 worker 输出到 stdout/stderr 的日志；不修改 `/v1/**`、`inspect`、SSE event、run trace、SQLite 中各类 `*_json` 字段的契约或存储格式。
- 本次目标不是取消结构化上下文，而是把当前 console encoder 末尾类似 JSON 的字段块改成人类可读的单行文本字段形式，同时保留结构化语义与稳定字段名。
- 本次改动继续复用现有 `pkg/logging` 作为统一日志入口，不更换日志框架，不引入新的日志依赖，也不增加文件落盘、远端收集或 metrics 系统。
- 本次默认采用单一推荐路径，不新增 `json|pretty` 双格式切换；日志级别仍沿用现有 `log_level`。
- 本次补日志遵循“最小可诊断”原则：优先输出链路阶段、关联 ID、结果、错误、耗时、数量和摘要，不默认输出完整 prompt、完整用户输入、完整工具入参与结果、鉴权口令、私有 memory 内容或其他敏感大字段。

## Pending Clarification

- 如果后续你希望一起修改 `inspect`/HTTP 查询接口的 JSON 输出形式，这应作为独立需求扩展到另一份 spec；本草案先把“日志都是 json 形式”解释为当前运行日志行尾字段可读性差的问题。

## Impacted Areas

- `pkg/logging/`: 统一日志格式、字段编码、调用约定与测试。
- `cmd/simiclaw/internal/gateway/`: 进程启动、配置装配和退出日志。
- `internal/http/{ingest,stream,middleware}/`: HTTP 写入口、流式入口、鉴权拒绝、连接生命周期和 handler 错误日志。
- `internal/gateway/`: 校验、binding、routing、幂等持久化、限流和 enqueue 结果日志。
- `internal/runtime/{eventloop,kernel,supervisor,workers}/`: 事件分发、claim、执行、finalize、恢复、定时任务与后台 worker 生命周期日志。
- `internal/runner/` 与 `internal/provider/`: payload plan、provider 调用、tool round、tool 执行和结果摘要日志。
- `internal/outbound/delivery/`: claim、发送、重试、dead-letter 和完成日志补齐。
- `internal/channels/telegram/`: 保持现有日志风格与字段约定一致，不单独发明另一套格式。
- `docs/references/configuration.md`: 如果日志行为说明发生变化，需要同步更新配置文档。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 运维能够从单行可读日志追踪一次 event 全链路 (Priority: P1)

作为运行维护者，我希望服务端日志是普通可读文本，而不是读起来像 JSON 片段的字段块，并且能通过同一组关联 ID 看清一条 event 从 ingest 到 finalize 再到 outbound 的全过程。

**Why this priority**: 这是本次需求的核心价值。如果日志仍然难读或关键链路仍然没有日志，后续排障效率不会实质改善。

**Independent Test**: 只实现这一故事时，团队就应该能在 `info` 级别下，通过一次正常请求看到“接收 -> 持久化/入队 -> 执行 -> 完成/失败 -> outbound”这些关键阶段，并且每条日志都带上足够的关联上下文。

**Acceptance Scenarios**:

1. **Given** 一次正常的 `POST /v1/events:ingest` 或 `POST /v1/chat:stream` 请求，**When** event 被接受并执行完成，**Then** 运维可以在日志中看到带 `event_id`、`session_key`、`session_id`、`run_id` 的关键阶段日志，且单行字段采用可读文本格式而不是 JSON 风格字段块。
2. **Given** 一次请求最终进入 outbound 发送，**When** 发送成功或失败，**Then** 运维可以用同一个 `event_id` 继续串联到对应 `outbox_id`、`channel`、`target_id`、发送结果与重试信息。

---

### User Story 2 - 开发者能够快速定位 runner/provider/tool 链路问题 (Priority: P1)

作为开发者，我希望 runner、provider 和 tool 执行阶段有足够的日志点，这样在模型超时、tool 调用失败、tool 轮次过多、payload 策略走错时，不需要只能依赖 trace 查询或断点调试。

**Why this priority**: 当前最难排查的问题集中在事务外执行链路，而这一段目前运行日志几乎是空白。

**Independent Test**: 只实现这一故事时，团队应该能通过一次 provider 超时、tool 失败或 `max_tool_rounds` 命中的场景，从日志直接看出失败发生在 runner 的哪个阶段，以及使用了哪个 model、哪个 tool、耗时多久。

**Acceptance Scenarios**:

1. **Given** provider 调用超时或返回错误，**When** run 失败，**Then** 日志必须显示 run 所属 `event_id/run_id`、provider、model、耗时、错误类型，并且错误只在最有上下文的一层被记录一次。
2. **Given** LLM 触发了 tool 调用，**When** tool 开始、结束、被策略拒绝或执行失败，**Then** 日志必须显示 `tool_call_id`、`tool_name`、摘要化的参数/结果大小信息，以及对应 run 的关联 ID。

---

### User Story 3 - 后台 worker 与恢复路径具备足够可观测性 (Priority: P2)

作为维护者，我希望 processing recovery、scheduled jobs、delivery worker、Telegram polling 等后台链路在成功、失败、空转和恢复时都有一致的日志，这样我能判断系统是在正常 idle、持续重试，还是已经卡住。

**Why this priority**: 当前后台 worker 大多只有零散错误日志，缺少“做了什么、为什么没做、做完结果如何”的上下文。

**Independent Test**: 只实现这一故事时，团队应该能通过人为制造 recovery、retry、cron ingest 失败等场景，从日志判断哪个 worker 在运行、回收了多少任务、是否成功重排队或进入 dead-letter。

**Acceptance Scenarios**:

1. **Given** 有处理超时的 event 需要 recovery，**When** sweeper 回收并尝试重新入队，**Then** 日志必须体现回收数量、重排队结果，以及受影响的 `event_id` 或计数摘要。
2. **Given** 有 delayed/retry/cron job 被 claim 并触发 ingest，**When** ingest 成功、重复、未入队或失败，**Then** 日志必须显示 `job_id`、job kind、生成的 `event_id`、是否 duplicate、是否 enqueued 与失败原因。

## Edge Cases

- 已持久化但未成功 enqueue 的 event，日志需要明确区分“接受成功但未入队”和“执行已开始”，避免误判。
- duplicate ingest、rate limit、invalid argument、API key 鉴权失败、binding/routing 失败需要有清晰日志，但不能把每类拒绝都打成 `ERROR`。
- `memory_flush`、`compaction`、`cron_fire` 等 `RunModeNoReply` payload 需要有运行日志，但不能误导为“用户可见回复”。
- provider 流式输出中断、客户端主动断开 SSE、tool 返回巨大结果、tool 参数或结果含敏感数据时，日志必须使用摘要或截断，而不是直接打印原文。
- 背景 worker 长时间空转时，不能在 `info` 级别持续刷屏；高频空转和内部调度细节应受 `debug` 级别约束。
- 同一个失败不能在 handler、gateway、runner、worker 多层重复打印 `ERROR`，避免一处故障放大成多条重复错误。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST emit human-readable single-line runtime logs for stdout/stderr, replacing the current JSON-like field blob style with readable text fields while retaining timestamp, level, caller, and message.
- **FR-002**: System MUST keep `pkg/logging` as the only shared logging entrypoint and MUST NOT reintroduce ad hoc `fmt.Print*`, raw `zap.L()` callsites, or package-local formatting conventions for runtime logs.
- **FR-003**: System MUST preserve structured context in log lines using stable field names, even when the rendered output is human-readable text rather than JSON-like fragments.
- **FR-004**: System MUST continue to honor the existing `log_level` configuration and gate high-frequency internal logs behind `debug`, while keeping `info` focused on lifecycle milestones and diagnostically useful state transitions.
- **FR-005**: System MUST NOT change HTTP response bodies, SSE payload schemas, persisted trace payloads, or SQLite schema fields as part of this logging upgrade.
- **FR-006**: System MUST standardize correlation fields where available, including `event_id`, `run_id`, `session_key`, `session_id`, `channel`, `payload_type`, `outbox_id`, `job_id`, `worker`, `tool_call_id`, `tool_name`, `provider`, and `model`.
- **FR-007**: System MUST log process lifecycle milestones, including service start, major subsystem startup failure, readiness-impacting failure, and graceful stop or worker shutdown boundaries where relevant.
- **FR-008**: System MUST log write ingress milestones for HTTP/channel entrypoints, including API-key authentication rejections, request decode/normalize failures, validation rejections, rate-limit rejections, binding/routing failures, successful persistence, duplicate detection, and enqueue outcome.
- **FR-009**: System MUST log event loop and runtime execution milestones: enqueue acceptance or drop, repump recovery summaries, claim success, execution start, finalize start, finalize completion, and finalize failure.
- **FR-010**: System MUST log runner execution summaries, including payload plan selection, run mode, provider invocation start and end, finish reason, tool-round count, token usage summary, latency, and terminal error outcome.
- **FR-011**: System MUST log tool execution start and finish, tool policy rejection, and tool execution failure, using sanitized argument/result summaries rather than raw large or sensitive payloads.
- **FR-012**: System MUST log background worker activity for processing recovery, scheduled jobs, and outbound delivery, including claim outcome, retry scheduling, dead-letter decisions, recovery counts, and terminal completion or failure.
- **FR-013**: System MUST classify log levels consistently: `INFO` for expected lifecycle or business milestones, `WARN` for rejected/retriable/degraded conditions, `ERROR` for terminal failures that require operator attention, and `DEBUG` for noisy internal state.
- **FR-014**: System MUST avoid duplicate terminal error logging across layers; a failure SHOULD be logged once at the layer that can attach the most actionable context, while lower layers return rich errors instead of logging again.
- **FR-015**: System MUST redact, summarize, or truncate sensitive or oversized data, including API keys, auth headers, Telegram tokens, full prompt bodies, private memory content, and large tool args/results.
- **FR-016**: System MUST keep the existing module-oriented message style, so logs remain attributable to subsystem owners such as `gateway`, `runtime.kernel`, `outbound.delivery`, and `telegram`.
- **FR-017**: System MUST define and test one canonical human-readable line shape so that representative logs across modules remain visually consistent and machine-greppable.
- **FR-018**: System MUST update repository documentation and tests for any user-visible logging behavior change, including configuration docs and logger output tests.

### Key Entities *(include if feature involves data)*

- **Runtime Log Line**: 一条输出到 stdout/stderr 的单行日志，包含固定前缀（时间、级别、caller、module/message）和稳定字段区。
- **Correlation Context**: 用于串联同一链路的标识集合，例如 `event_id`、`run_id`、`session_key`、`outbox_id`、`job_id`、`tool_call_id`。
- **Milestone Log**: 表示一次链路进入关键阶段的日志，例如 ingest accepted、run started、provider completed、tool failed、finalize completed、outbound sent。
- **Sanitized Summary**: 经过去敏、截断或计数归纳后的字段输出形式，用于替代直接打印原始 prompt、tool payload 或私有内容。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 在 `info` 级别下，一次正常 event 的主链路至少能通过日志看清 `ingest accepted -> runtime started -> finalize completed -> outbound sent/skip` 这几个关键阶段，且能通过关联字段串起来。
- **SC-002**: 在 provider 超时、tool 失败、duplicate ingest、rate limit、outbound retry 这几类代表性异常场景中，每类至少存在一条带完整上下文的高价值日志，能够直接指出失败所在层和关键关联 ID。
- **SC-003**: 新日志格式不再以 JSON-like 字段块作为主要展示方式；代表性 logger 测试能够验证输出为人类可读单行文本字段格式。
- **SC-004**: 现有 `log_level` 行为保持有效；`info` 不出现高频空转刷屏，而 `debug` 能额外暴露调度、中间阶段与摘要性内部状态。
- **SC-005**: 代表性测试或人工验证中，不出现 API key、Bearer token、Telegram token、完整 prompt 正文、private memory 内容或未截断的大型 tool payload 被直接打印到日志。
- **SC-006**: 本次改动后，仓库中日志调用仍收敛到统一 logging 入口，且不会引入新的打印式日志 tech debt。
