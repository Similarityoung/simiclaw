# Documentation Tech Debt Tracker

## Purpose

记录当前文档体系尚未覆盖、但已经能从代码中看见需求的空白。这里不替代 issue，而是作为仓库内的文档治理清单。

## Open Items

| Area | Current State | Evidence | Suggested Next Step |
| --- | --- | --- | --- |
| API 示例 | 已有 HTTP 路由说明，但没有稳定请求/响应样例 | `internal/httpapi/routes.go`, `pkg/api/` | 增加面向操作的 curl / JSON 示例文档 |
| 数据库可读文档 | `schema.sql` 是唯一完整事实源 | `internal/store/schema.sql` | 生成 `docs/generated/db-schema.md` 或等价视图 |
| 前端架构 | `web/` 已有多页面与状态管理，但没有说明文档 | `web/src/` | 补前端路由、状态与 API 依赖说明 |
| Telegram 运维 | 配置入口和部分测试已存在，缺单独操作手册 | `internal/config/config.go`, `tests/integration/telegram_integration_test.go` | 写最小接入与故障排查文档 |
| 文档校验 | 目前只有人工检查 | `docs/`, `README.md`, `AGENTS.md` | 在 CI 中加入链接检查或结构校验 |

## Machine Snapshot

<!-- BEGIN:CI_TECH_DEBT -->
_This block is maintained by repo hygiene. Do not edit it by hand._

| Rule | New | Existing | Shrink Candidates |
| --- | --- | --- | --- |
| context-background | 0 | 2 | 0 |
| file-lines | 0 | 7 | 0 |
| go-statement | 0 | 12 | 0 |
| name-token | 0 | 2 | 0 |
| panic-call | 0 | 3 | 0 |
| print-logging | 0 | 1 | 0 |

### Shrink Candidates

- No shrink candidates in this run.

### Docs Links

- Status: unknown
<!-- END:CI_TECH_DEBT -->

## Notes

- 新增专题文档前，优先先补索引链接，确保读者能从 `docs/index.md` 找到它。
- 如果某项内容将来能自动生成，优先放进 `docs/generated/`，避免手写说明与源码双份维护。
