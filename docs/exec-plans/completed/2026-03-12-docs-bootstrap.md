# 2026-03-12 文档系统建设

## Objective

为 SimiClaw 建立一套“入口地图 -> 索引 -> 细节文档”的文档系统，减少 `AGENTS.md` / `README.md` 的重复负担，并把可核实事实收敛到 `docs/`。

## Scope

- 包含:
  - 盘点现有入口文档与代码事实源
  - 新建 `ARCHITECTURE.md` 和 `docs/` 导航体系
  - 为运行链路、模块边界、prompt/workspace、配置、测试、工作区布局补最小可用文档
  - 瘦身根部 `AGENTS.md` 与 `README.md`
- 不包含:
  - 自动生成数据库 schema 文档
  - 前端深度专题文档
  - CI 级文档检查脚本

## Plan

1. 阅读 `AGENTS.md`、`README.md`、Makefile、配置代码、runtime 主链路和架构测试，确认权威来源。
2. 设计 `docs/` 的信息架构，并为这次改造建立执行记录。
3. 创建设计文档、参考文档和质量评分，迁移入口文件内容。
4. 校验链接和目录结构，记录剩余缺口。

## Progress Log

- 2026-03-12: 盘点根部文档、命令、配置、schema、runtime 主链路和架构测试。
- 2026-03-12: 新建 `ARCHITECTURE.md`、`docs/index.md`、`docs/design-docs/*`、`docs/references/*`、`docs/QUALITY_SCORE.md`。
- 2026-03-12: 将根部 `AGENTS.md` 收敛为导航图，将 `README.md` 收敛为用户向入口。

## Decision Log

- 2026-03-12: 保留 `ARCHITECTURE.md` 在仓库根部，作为 `README.md` 与 `AGENTS.md` 都能直接指向的总览入口。
- 2026-03-12: 不直接把旧 README 原样拆散复制，而是以代码为准重写最小必要文档，避免把未经复核的信息继续扩散。
- 2026-03-12: 为 `docs/exec-plans/active/` 增加占位入口，确保后续复杂改动有稳定落点。

## Risks

- `web/` 前端的文档覆盖仍然偏薄，后续如果前端变化频繁，README 与架构总览可能再次显得过于概括。
- 数据库 schema 目前只有源码级权威来源，缺一份更面向人的可浏览文档。
- 仓库尚未接入文档链接校验，后续改名时可能出现静态链接漂移。

## Exit Criteria

- `AGENTS.md` 保持短小且只做导航。
- `README.md` 能提供最小可用上手路径并指向深入文档。
- `docs/` 至少覆盖架构、运行链路、模块边界、配置、测试和工作区布局。
- 剩余缺口被记录在技术债或质量评分中，而不是继续隐藏在聊天上下文里。
