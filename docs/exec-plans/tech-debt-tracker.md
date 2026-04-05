# Documentation Tech Debt Tracker

## Purpose

记录当前文档体系尚未覆盖、但已经能从代码中看见需求的空白。这里不替代 issue，而是作为仓库内的文档治理清单。

## Open Items

| Area | Current State | Evidence | Suggested Next Step |
| --- | --- | --- | --- |
| API 示例 | 已有 HTTP 路由说明，但没有稳定请求/响应样例 | `internal/http/server.go`, `internal/http/{ingest,query,stream}/`, `pkg/api/` | 增加面向操作的 curl / JSON 示例文档 |
| 数据库可读文档 | `schema.sql` 是唯一完整事实源 | `internal/store/schema.sql` | 生成 `docs/generated/db-schema.md` 或等价视图 |
| 前端架构 | `web/` 已有多页面与状态管理，但没有说明文档 | `web/src/` | 补前端路由、状态与 API 依赖说明 |
| Telegram 运维 | 配置入口和部分测试已存在，缺单独操作手册 | `internal/config/config.go`, `tests/integration/telegram_integration_test.go` | 写最小接入与故障排查文档 |
| 文档校验 | 目前只有人工检查 | `docs/`, `README.md`, `AGENTS.md` | 在 CI 中加入链接检查或结构校验 |

## Machine Snapshot

<!-- BEGIN:CI_TECH_DEBT -->
_This block is maintained by repo hygiene. Do not edit it by hand._

| Rule | New | Existing | Shrink Candidates |
| --- | --- | --- | --- |
| context-background | 1 | 1 | 1 |
| file-lines | 2 | 4 | 3 |
| go-statement | 3 | 6 | 6 |
| name-token | 0 | 2 | 0 |
| panic-call | 1 | 2 | 1 |
| print-logging | 0 | 1 | 0 |

### Shrink Candidates

- cmd/simiclaw/internal/chat/tui.go:1 — production Go file is 801 lines (> 600)
- cmd/simiclaw/internal/chat/tui.go:713 — review go statement ownership and shutdown behavior
- internal/runtime/workers.go:82 — review go statement ownership and shutdown behavior
- internal/runtime/workers.go:83 — review go statement ownership and shutdown behavior
- internal/runtime/workers.go:84 — review go statement ownership and shutdown behavior
- internal/runtime/workers.go:85 — review go statement ownership and shutdown behavior
- internal/runtime/workers.go:86 — review go statement ownership and shutdown behavior
- internal/runtime/workers.go:87 — avoid new Background() in production code
- internal/store/events.go:1 — production Go file is 413 lines (warning range 401-600)
- internal/systemprompt/system.go:40 — review panic usage in production code
- internal/workspacefile/workspacefile.go:1 — production Go file is 417 lines (warning range 401-600)

### Docs Links

- Status: failure

~~~text
# Summary

| Status         | Count |
|----------------|-------|
| 🔍 Total       | 77    |
| ✅ Successful  | 0     |
| ⏳ Timeouts    | 0     |
| 🔀 Redirected  | 0     |
| 👻 Excluded    | 74    |
| ❓ Unknown     | 0     |
| 🚫 Errors      | 3     |
| ⛔ Unsupported | 0     |

## Errors per input

### Errors in docs/design-docs/runtime-kernel-refactor.md

* [ERROR] <error:> | Error building URL for "/Users/similarityyoung/Documents/SimiClaw/specs/001-runtime-kernel-refactor/plan.md" (Attribute: Some("href")): Cannot convert path '/Users/similarityyoung/Documents/SimiClaw/specs/001-runtime-kernel-refactor/plan.md' to a URI: To resolve root-relative links in local files, provide a root dir
* [ERROR] <error:> | Error building URL for "/Users/similarityyoung/Documents/SimiClaw/specs/001-runtime-kernel-refactor/spec.md" (Attribute: Some("href")): Cannot convert path '/Users/similarityyoung/Documents/SimiClaw/specs/001-runtime-kernel-refactor/spec.md' to a URI: To resolve root-relative links in local files, provide a root dir
* [ERROR] <error:> | Error building URL for "/Users/similarityyoung/Documents/SimiClaw/specs/001-runtime-kernel-refactor/tasks.md" (Attribute: Some("href")): Cannot convert path '/Users/similarityyoung/Documents/SimiClaw/specs/001-runtime-kernel-refactor/tasks.md' to a URI: To resolve root-relative links in local files, provide a root dir
~~~
<!-- END:CI_TECH_DEBT -->

## Notes

- 新增专题文档前，优先先补索引链接，确保读者能从 `docs/index.md` 找到它。
- 如果某项内容将来能自动生成，优先放进 `docs/generated/`，避免手写说明与源码双份维护。
