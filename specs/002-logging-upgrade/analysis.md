# Analysis: 日志系统升级与链路观测补齐

## Purpose

本分析用于回答三个问题：

- 当前“日志少且读起来像 JSON 片段”的问题，是否可以在不改 runtime 契约的前提下解决
- 这次日志升级的 owner 应该放在哪些层，哪些层不应承担终态日志责任
- 按现有仓库形状，最短正确实现路径是什么，哪些做法会把问题做大

## Current Baseline

先看当前代码基线，可以得到几个关键事实：

### 1. 统一日志入口已经存在，但输出形状还停留在 zap console 默认字段串

- `pkg/logging/logger.go` 已经统一封装了 `Init`、`L(module)`、`With(...)` 和 caller 输出。
- `pkg/logging/logger_test.go` 当前断言的字段区仍是类似 `"key": "value"` 的 JSON-like 片段，而不是稳定的 `key=value` 文本。
- 这说明“统一入口”不是问题，问题主要在渲染形状和调用点覆盖不足。

结论：最短路径不是换库，而是在 `pkg/logging` 内固定新 line shape，再补调用点。

### 2. 当前日志覆盖高度不均匀

- `internal/runtime/kernel/service.go`、`internal/outbound/delivery/worker.go`、`internal/channels/telegram/runtime.go` 已经有少量结构化日志。
- `internal/gateway/service.go` 是 ingest 写路径核心，但当前几乎完全无日志。
- `internal/http/ingest/handler.go`、`internal/http/stream/handler.go` 只有错误响应，没有运行日志。
- `internal/runtime/workers/processing_recovery.go` 与 `internal/runtime/workers/scheduled_jobs.go` 当前基本 silent。
- `internal/runner/runner.go` 与 `internal/provider/openai*.go` 负责最难排障的事务外执行链路，但当前几乎没有运行日志。

结论：这次升级的重点不在“让已有日志更好看”，而在“把目前空白的主链路补齐到最小可诊断”。

### 3. ingress 真正的入口不只是在 handler 内

- `internal/http/server.go` 先经过 `internal/http/middleware/api_key.go`，然后才进入 ingest/stream handler。
- 当 `cfg.APIKey` 开启时，401 会在 middleware 直接返回，根本不会进入 `internal/http/{ingest,stream}`。

结论：如果只在 handler 和 gateway 补日志，API key 拒绝仍然会是观测盲区；它必须被视为 ingress milestone 的一部分。

### 4. provider 层不具备终态日志 owner 所需的关联上下文

- `internal/provider/openai.go` 和 `internal/provider/openai_stream.go` 只拿到 `context.Context` 与 `ChatRequest`。
- `event_id`、`run_id`、`session_id` 等链路关联字段是在 `internal/runner/runner.go` 这一层才天然齐备。
- `ARCHITECTURE.md` 和 `runtime-flow.md` 也明确了 runner 才是 provider/tools 的执行编排 owner。

结论：provider 可以提供底层 transport 诊断，但终态 provider 失败日志必须归 runner；否则要么缺关联 ID，要么重复记错。

## What The Existing Shape Gets Right

虽然当前日志体验不够好，但仓库现状其实已经给出了正确方向：

### 1. 模块 owner 已经初步存在

现有日志调用基本都通过 `logging.L("module")` 写出，如：

- `cmd`
- `runtime.kernel`
- `outbound.delivery`
- `telegram`

这意味着本次不需要再发明“统一日志上下文框架”，只需要把缺失模块继续按 owner 补齐。

### 2. runtime 不变量天然提供了日志里程碑

根据 `ARCHITECTURE.md` 与 `docs/design-docs/runtime-flow.md`，主链路已经天然分成：

- ingress 接收
- persist / enqueue
- claim / started run
- runner / provider / tools
- finalize
- outbound / stream

这组阶段本身就是最适合作为 milestone log 的骨架，不需要额外设计一套新的生命周期模型。

### 3. 包边界已经足够明确

`docs/design-docs/module-boundaries.md` 和 architecture tests 已经限制了 `http`、`runtime`、`runner`、`channels` 不能直接穿透 `store`。

这对日志升级有一个直接影响：

- 日志应当在本层用本层已知事实记录
- 不应为了“字段更全”引入跨层查询、旁路拿 store、或额外拼装全局上下文对象

## What We Should Not Do

结合当前仓库约束，这次日志升级不应该走以下路径：

### 1. 不要把它做成 observability 基建重构

不需要新增：

- 新日志框架
- `json|pretty` 双格式切换
- 全局 context logger/middleware 框架
- 独立 metrics/tracing collector
- 新的 observability 顶层包

这些都会扩大 scope，但并不能更快解决“读不懂日志”和“关键链路没日志”的问题。

### 2. 不要把所有层都变成终态错误 logger

一条失败如果同时在：

- handler
- gateway
- runner
- worker
- provider

各打印一次 `ERROR`，最终得到的是噪音，而不是可诊断性。

这里应坚持现有仓库已经在 `simiclaw-repo-dev` 里强调的规则：最有上下文的一层记录一次，其他层返回 rich error。

### 3. 不要为了“多暴露些日志”而直接打印原文

当前需求覆盖了：

- prompt
- private memory
- Authorization / API key
- Telegram token
- tool args / tool results

这些数据一旦直接打进日志，收益有限，风险很高。正确方向不是“尽量多打印”，而是“默认摘要化，只暴露可诊断最小集”。

### 4. 不要把 worker idle 和 stream keepalive 提升到 `info`

`processing_recovery`、`scheduled_jobs`、SSE keepalive 都是高频链路。

如果每个 tick、每次空转、每次 keepalive 都进 `info`，主日志面会再次被噪声淹没。对运维真正有价值的是：

- claim 了什么
- 回收了多少
- 重试还是 dead-letter
- 失败发生在哪一层

而不是“又轮询了一次”。

## Fit Against The Current Speckit Plan

对照当前 `spec.md` 与 `plan.md`，我的判断是：

### 已经对齐的点

- Scope 只覆盖 stdout/stderr 运行日志，不改 `/v1/**`、SSE、trace、SQLite JSON 字段契约。
- 推荐路径保持在 `pkg/logging` + 精准调用点调整，没有引入兼容层和额外依赖。
- `info` / `debug` 分层目标明确，符合“最小可诊断”原则。
- 文档、测试、脱敏和回归验证已经被纳入同一 feature，而不是留作后补。

### 需要显式守住的点

- ingress 范围必须包含 API key 鉴权拒绝，而不只是 handler 内 decode/normalize/validate 失败。
- provider 终态错误日志 owner 必须放在 runner，而不是 provider。
- worker 侧日志应优先写“数量、结果、决策摘要”，而不是逐条 tick 噪声。
- quickstart 和验证步骤必须使用实时 timestamp，并注明鉴权头，否则验证文档会很快失效。

## Recommended Adaptation For This Feature

基于当前仓库形状，这次 feature 最稳妥的实现骨架应当是：

### 1. 先固定 logger line shape，再扩调用点

第一阶段只做一件事：

- 把 `pkg/logging` 的字段区从 JSON-like blob 收敛为稳定 `key=value`

并用 logger 单测固定：

- 时间
- 级别
- caller
- `[module] message`
- 字段转义
- 错误字段
- 空值 / 布尔 / 数值

这样后续补点时，所有模块天然共享同一视觉契约。

### 2. 把日志责任严格放在“拥有该阶段语义”的层

推荐 owner 划分如下：

- `http/middleware`: 认证拒绝
- `http/ingest` / `http/stream`: 请求解码、协议级失败
- `gateway`: validate / binding / routing / persist / duplicate / enqueue
- `runtime.kernel` / `eventloop`: claim / execute start / finalize / repump 摘要
- `runner`: payload plan、provider 开始/结束/失败、tool round summary、terminal outcome
- `provider`: 仅保留必要的底层 transport 诊断，优先 `debug`
- `outbound.delivery`: claim / send / retry / dead-letter / complete
- `workers`: recovery / scheduled job 的数量和结果摘要
- `telegram`: 继续保留 channel owner 风格，不发明新语义

### 3. 把“字段完整性”改成“有则记录”，不要反向造数据

`spec.md` 已经把 correlation fields 定义为 “where available”。这点必须坚持。

正确做法是：

- 本层已有 `event_id/run_id/session_id` 就记录
- 本层只有 `job_id`、`worker`、`channel` 就先记录这些
- 不为了字段对齐去跨层查库或新增上下文透传链条

这样既满足 grep 和人工扫描，也不破坏现有边界。

### 4. 以代表性场景验证，不做脆弱的全链路日志 golden

从当前测试矩阵看，最合适的验证组合是：

- `go test ./pkg/logging/...` 固定渲染契约
- 包级 targeted tests 覆盖代表性日志点与脱敏规则
- `go test ./tests/architecture/... -v` 确认没破坏边界
- `make test-unit`
- `make accept-current`

不建议把整条链路做成大段完整日志 golden，因为那会让测试对消息措辞和字段顺序过于脆弱。

## Impact On The Current Speckit Flow

基于上述分析，对当前 feature 的 speckit 流程建议是：

1. `spec.md` 保持“运行日志升级而非契约改造”的 scope 不变。
2. `plan.md` 继续按 `pkg/logging -> ingress/runtime/outbound -> runner/provider/workers -> docs/tests` 这条顺序推进。
3. `tasks.md` 在拆任务时，必须把下列事项显式写成任务，而不是留给实现时自由发挥：
   - logger canonical line shape 与测试
   - API key 拒绝日志
   - runner 作为 provider 终态日志 owner
   - worker 摘要日志与空转噪声控制
   - 脱敏/截断规则与 targeted tests
   - quickstart / configuration / testing 文档同步

## Conclusion

这次日志升级不需要新框架，也不需要改 runtime 语义。最短正确路径就是：

- 保留 `pkg/logging` 作为唯一入口
- 把输出形状改成人类可读单行
- 按现有 runtime 分层给真正缺失的 milestone 补点
- 让 runner 而不是 provider 持有终态 provider 失败日志
- 让 ingress 覆盖到 middleware 鉴权拒绝
- 用摘要与级别分层控制敏感信息和噪声

只要守住这些边界，这个 feature 可以在不扩大仓库结构的前提下，明显提升主链路排障效率。
