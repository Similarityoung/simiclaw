# Documentation Quality Score

## Scoring Rubric

- `0`: 缺失
- `1`: 有文档但不可信，事实难以回溯
- `2`: 基本可用，但覆盖不全或缺少交叉链接
- `3`: 结构清晰、来源可核实，仍有明显空白
- `4`: 结构完整、持续维护、具备自动校验

## Domain Scores

| Domain | Score | Evidence | Gaps | Next Step |
| --- | --- | --- | --- | --- |
| Architecture | 3 | `ARCHITECTURE.md`, `docs/design-docs/runtime-flow.md`, `tests/architecture/boundaries_test.go` | 缺少数据库 schema 的可读化专题文档 | 补 `docs/generated/db-schema.md` 或等价生成物 |
| Prompt / Workspace | 3 | `docs/design-docs/prompt-and-workspace-context.md`, `docs/references/workspace-layout.md` | 缺具体调试示例 | 增加常见 run 场景示例 |
| Configuration / Ops | 2 | `docs/references/configuration.md`, `Makefile` | 缺部署、监控、Telegram 运维说明 | 增加 operations 文档 |
| Testing | 3 | `docs/references/testing.md`, `Makefile`, `tests/` | 仍缺“什么改动跑哪套测试”的决策表 | 增补变更类型到测试建议的映射 |
| Frontend | 1 | README 仅保留入口，`web/` 代码存在 | 缺前端架构、路由和状态文档 | 补 `web/` 专题说明 |
| Documentation Governance | 2 | `docs/index.md`, `docs/exec-plans/`, 本评分文档 | 还没有 CI 链接检查和周期性养护机制 | 加入 docs lint / link check |

## Machine Snapshot

<!-- BEGIN:CI_QUALITY_SCORE -->
_This block is maintained by repo hygiene. Do not edit it by hand._

| Metric | Value |
| --- | --- |
| Last Run (UTC) | 2026-03-12T12:42:18Z |
| Guardrails New | 0 |
| Guardrails Existing | 27 |
| Shrink Candidates | 0 |
| Warning Hotspots | 19 |
| Docs Links | unknown |

### Top Rules

- go-statement: 12
- file-lines: 7
- panic-call: 3
- context-background: 2
- name-token: 2
<!-- END:CI_QUALITY_SCORE -->

## Notes

- 本评分优先依据代码、测试、配置与目录结构，而不是旧 README 里的叙述。
- `Frontend` 分数偏低不是因为实现不存在，而是因为文档覆盖明显落后于代码现状。
- 目标不是一次把所有分数拉满，而是先把入口、索引和权威来源稳定下来。
