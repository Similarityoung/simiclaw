# 文档索引

## Purpose

`docs/` 是 SimiClaw 的记录系统，用来承载长期可维护的设计说明、参考资料、执行计划和文档治理结果。根部 `README.md` 与 `AGENTS.md` 只做入口，不再承载全部细节。

## Read This First

- [`../README.md`](../README.md): 用户与开发者的最小起步路径
- [`../ARCHITECTURE.md`](../ARCHITECTURE.md): 系统结构总览、主数据流和硬约束
- [`design-docs/runtime-flow.md`](design-docs/runtime-flow.md): 从 ingest 到 finalize 的执行链路
- [`references/configuration.md`](references/configuration.md): 配置、环境变量、鉴权与 CLI 运行参数

## 文档域

### 设计文档

- [`design-docs/index.md`](design-docs/index.md): 设计文档导航页
- [`design-docs/core-beliefs.md`](design-docs/core-beliefs.md): V1 设计原则与取舍
- [`design-docs/runtime-flow.md`](design-docs/runtime-flow.md): runtime 主链路
- [`design-docs/module-boundaries.md`](design-docs/module-boundaries.md): 由架构测试强制执行的模块边界
- [`design-docs/prompt-and-workspace-context.md`](design-docs/prompt-and-workspace-context.md): prompt、workspace、skills、memory 与写文件边界

### 参考资料

- [`references/configuration.md`](references/configuration.md): 服务端配置、环境变量和 HTTP 鉴权
- [`references/current-prompt-templates.md`](references/current-prompt-templates.md): 当前生效的提示词模板、动态 section 和运行时变体
- [`references/testing.md`](references/testing.md): 测试矩阵、命令和阶段验收入口
- [`references/workspace-layout.md`](references/workspace-layout.md): 工作区布局、scaffold 文件和路径约束

### 计划与治理

- [`exec-plans/active/README.md`](exec-plans/active/README.md): 当前活跃执行计划入口
- [`exec-plans/completed/2026-03-12-docs-bootstrap.md`](exec-plans/completed/2026-03-12-docs-bootstrap.md): 本次文档系统建设记录
- [`exec-plans/tech-debt-tracker.md`](exec-plans/tech-debt-tracker.md): 仍待补齐的技术与文档债
- [`QUALITY_SCORE.md`](QUALITY_SCORE.md): 当前文档覆盖度和缺口评分

## 权威来源

- 命令与测试: `Makefile`, `VERSION_STAGE`
- 运行时主链路: `internal/bootstrap/app.go`, `internal/gateway/service.go`, `internal/runtime/eventloop.go`, `internal/runtime/kernel/service.go`, `internal/runner/runner.go`
- 存储与 schema: `internal/store/db.go`, `internal/store/schema.sql`
- 边界约束: `tests/architecture/boundaries_test.go`
- Prompt / workspace: `internal/prompt/*.go`, `internal/workspace/scaffold.go`, `internal/workspacefile/workspacefile.go`

## Status

- 已验证: 架构分层、运行时链路、配置入口、测试矩阵、工作区布局
- 待补全: API 请求/响应示例、前端架构说明、数据库 schema 的自动生成文档
- 待治理: 文档链接校验与周期性 doc-gardening 还没有进入 CI

## Next

- 如果你要理解系统怎么跑，先读 [`../ARCHITECTURE.md`](../ARCHITECTURE.md)。
- 如果你要改 runtime，接着读 [`design-docs/runtime-flow.md`](design-docs/runtime-flow.md)。
- 如果你要改模块依赖，先读 [`design-docs/module-boundaries.md`](design-docs/module-boundaries.md)。
- 如果你要改工作区提示、skills、memory 或文件写工具，读 [`design-docs/prompt-and-workspace-context.md`](design-docs/prompt-and-workspace-context.md)。
- 如果你要直接优化提示词文案或注入格式，读 [`references/current-prompt-templates.md`](references/current-prompt-templates.md)。
