# Module Boundaries

## Summary

SimiClaw 的分层不是“建议”，而是被架构测试直接校验的硬约束。读这篇文档时，应把 `tests/architecture/boundaries_test.go` 看成权威来源，而不是把这里当成新的事实源。

## Context

仓库采用 `internal/<subsystem>` 的方式拆分子系统，但真正重要的是依赖方向和契约稳定性。V1 当前通过一组 architecture tests 来阻止常见的层间穿透。

## Details

| 规则 | 说明 | 证据 |
| --- | --- | --- |
| 非存储层不得直接依赖 `internal/store` | `http`、`channels`、`workspace`、`query`、`runner`、`runtime` 都被禁止直接 import store | `TestHTTPProductionCodeDoesNotImportStore` 等 |
| `internal/readmodel` 只供 store 内部使用 | 生产代码中，除 `internal/store` 外不得 import `internal/readmodel` | `TestNonStoreProductionCodeDoesNotImportReadModel` |
| `pkg/api` 是对外稳定契约 | `internal/store`、`internal/query/model`、`internal/ingest/port` 不得反向依赖 `pkg/api` | `TestStoreProductionCodeDoesNotImportAPI` 等 |
| 每个子系统自己的 DTO 放在自己的 `model/` | `internal/query/model`、`internal/runner/model`、`internal/runtime/model` 不承诺对外稳定 | 目录结构与测试意图 |
| 只有 ingest 入口能直接触发 `IngestEvent` | 避免系统里出现多个写路径入口 | `TestOnlyIngestServiceCallsIngestEventOutsideTests` |
| CLI 命令统一走 Cobra | 命令包不能 import 标准库 `flag` | `TestCommandPackagesDoNotImportStdlibFlag` |
| chat TUI 不直接依赖 `net/http` / `net/url` | 让命令层通过共享 client 访问服务 | `TestChatProductionCodeDoesNotImportNetHTTP`, `TestChatProductionCodeDoesNotImportNetURL` |

### 为什么这些规则重要

- 它们让 `bootstrap.NewApp` 仍然是主要装配点，而不是让各个子系统随意拿着 `*store.DB` 到处透传。
- 它们逼着查询层、执行层和渠道层使用更小的接口，从而减少“因为一个表字段变更导致整串包一起改”的连锁反应。
- 它们也让 `pkg/api` 能继续作为稳定的 HTTP / SSE / CLI wire model，而不是被内部存储细节拖着走。

## Verification

- 架构测试: `tests/architecture/boundaries_test.go`
- 应用装配: `internal/bootstrap/app.go`
- HTTP 路由: `internal/http/server.go`
- CLI 根命令: `cmd/simiclaw/internal/root/command.go`

## Related Docs

- 系统总览: [`../../ARCHITECTURE.md`](../../ARCHITECTURE.md)
- 运行链路: [`runtime-flow.md`](runtime-flow.md)
- 配置参考: [`../references/configuration.md`](../references/configuration.md)
- 测试矩阵: [`../references/testing.md`](../references/testing.md)
