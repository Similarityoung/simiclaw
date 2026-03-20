# Research: 日志系统升级与链路观测补齐

## Decision 1 - 保留 `pkg/logging` + zap，不更换日志库

- **Decision**: 继续复用 `pkg/logging` 和 zap，只替换当前 console 输出渲染方式与调用点覆盖。
- **Why**: 这是满足“日志更像正常文本”需求的最短路径，同时能保留现有 `Field`/`With`/caller/module 约定，避免全仓大面积改签名。

## Decision 2 - 不新增 `log_format` 双格式切换

- **Decision**: 本次直接把默认运行日志切到人类可读单行格式，不新增 `json|pretty` 切换。
- **Why**: 用户当前明确要“正常日志形式”，仓库也没有现成双格式需求；增加切换会扩大配置面和测试面，不符合最短正确路径。

## Decision 3 - 范围只覆盖运行日志，不改查询/trace JSON 契约

- **Decision**: 本次不修改 `/v1/**`、`inspect`、SSE、run trace 或 SQLite 中 `*_json` 字段的格式。
- **Why**: 这些接口和存储承担的是程序契约或查询事实，不属于“stdout/stderr 运行日志可读性”问题；把它们混进来会把日志优化变成接口改造。

## Decision 4 - 默认输出摘要与脱敏，而不是原文

- **Decision**: prompt、私有 memory、鉴权信息、tool 原始大参数和结果不直接打印，统一走摘要、截断或显式脱敏。
- **Why**: 日志覆盖面一旦扩大，泄漏风险会同步上升；默认摘要化是“多暴露链路”与“不过度暴露内容”之间的必要边界。

## Decision 5 - `info` 记录里程碑，`debug` 记录高频中间态

- **Decision**: `info` 只保留运维与排障最关键的 milestone；空轮询、keepalive、逐 chunk/逐 tick 细节压回 `debug`。
- **Why**: 需求是“日志更多且更好读”，不是“任何内部动作都写到 `info`”；如果主日志面被噪声淹没，可读性会再次下降。
