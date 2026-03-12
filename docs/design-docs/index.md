# Design Docs Index

## Purpose

这里收纳 SimiClaw 的设计原则、运行链路和模块边界。它关注“为什么这样分层”和“哪些约束是代码里真正 enforce 的”，不重复快速开始或命令说明。

## Read This First

- [`core-beliefs.md`](core-beliefs.md): V1 的设计信念与不愿妥协的约束
- [`runtime-flow.md`](runtime-flow.md): event 从进入系统到提交结果的主路径
- [`module-boundaries.md`](module-boundaries.md): 哪些依赖关系被架构测试直接卡死
- [`prompt-and-workspace-context.md`](prompt-and-workspace-context.md): prompt、memory、skills 与 workspace 文件边界

## Status

- 已验证:
  - `core-beliefs.md`
  - `runtime-flow.md`
  - `module-boundaries.md`
  - `prompt-and-workspace-context.md`
- 待补全:
  - API 示例与常见请求流程图
  - 前端路由、状态管理与后端协作方式
  - 数据库 schema 的可读化专题文档
- 待复核:
  - Telegram 运营层面的说明目前只做了最小覆盖，仍缺单独操作手册

## Related Docs

- 系统总览: [`../../ARCHITECTURE.md`](../../ARCHITECTURE.md)
- 参考资料: [`../references/configuration.md`](../references/configuration.md)
- 工作区布局: [`../references/workspace-layout.md`](../references/workspace-layout.md)
- 文档质量评分: [`../QUALITY_SCORE.md`](../QUALITY_SCORE.md)
