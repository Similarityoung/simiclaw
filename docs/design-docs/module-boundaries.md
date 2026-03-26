# Module Boundaries

## Summary

SimiClaw 的分层不是“建议”，而是被架构测试直接校验的硬约束。`spec03` 之后，这些约束默认按四块 owner map 来理解：`Surface`、`Runtime`、`Context/State`、`Capability Plane`。读这篇文档时，应把 `tests/architecture/*.go` 看成权威来源，而不是把这里当成新的事实源。

## Context

仓库采用 `internal/<subsystem>` 的方式拆分子系统，但真正重要的是依赖方向和契约稳定性，而不是目录名本身。V1 当前通过一组 architecture tests 来阻止常见的层间穿透，并把 major module 固定到四块 owner 中。

## Details

| 规则 | 说明 | 证据 |
| --- | --- | --- |
| 非存储层不得直接依赖 `internal/store` | `http`、`channels`、`workspace`、`query`、`runner`、`runtime` 都被禁止直接 import store | `TestHTTPProductionCodeDoesNotImportStore` 等 |
| Surface adapter 只允许走 Runtime / Query seam | `internal/http`、`internal/channels` 不得直接 import context assets 或 capability plane | `TestSurfaceAdaptersDoNotImportContextAssets`, `TestSurfaceAdaptersDoNotImportCapabilityPlane` |
| Runtime 不反向依赖 Surface | `internal/gateway`、`internal/runtime`、`internal/outbound` 不得 import `internal/http`、`internal/channels` 或 CLI command 包 | `TestRuntimePlaneProductionCodeDoesNotImportSurfaceAdapters` |
| Runtime 对 Context/State 与 Capability 的消费需要收口 | 当前最小 guardrail 是 `gateway/runtime/outbound` 不直接 import `query/prompt/memory/workspace*/provider/tools`，这些依赖应集中在更明确的 owner | `TestRuntimeCoreProductionCodeDoesNotImportContextStatePlane`, `TestRuntimeCoreProductionCodeDoesNotImportCapabilityPlane` |
| Query 不反向定义 Surface 行为 | `internal/query` 不得 import HTTP/channel/CLI surface adapter | `TestQueryProductionCodeDoesNotImportSurfaceAdapters` |
| `pkg/api` 是对外稳定契约 | `internal/store`、`internal/query/model`、`internal/channels` 不得反向依赖 `pkg/api` | `TestStoreProductionCodeDoesNotImportAPI` 等 |
| 每个子系统自己的 DTO 放在自己的 `model/` | `internal/query/model`、`internal/runner/model`、`internal/runtime/model` 不承诺对外稳定 | 目录结构与测试意图 |
| 只有 gateway 写入口能直接触发 `PersistEvent` | 避免系统里出现多个写路径入口 | `TestOnlyGatewayServiceCallsPersistEventOutsideTests` |
| CLI 命令统一走 Cobra | 命令包不能 import 标准库 `flag` | `TestCommandPackagesDoNotImportStdlibFlag` |
| chat TUI 不直接依赖 `net/http` / `net/url` | 让命令层通过共享 client 访问服务 | `TestChatProductionCodeDoesNotImportNetHTTP`, `TestChatProductionCodeDoesNotImportNetURL` |
| owner map 不能重新塌成混合中心对象 | 同一模块不能同时承担 `transport + execution + observe + fallback` | `TestFourPlaneOwnerMapDoesNotCollapseTransportExecutionObserveAndFallback` |

### 为什么这些规则重要

- 它们让 `bootstrap.NewApp` 仍然是主要装配点，而不是让各个子系统随意拿着 `*store.DB` 到处透传。
- 它们逼着查询层、执行层和渠道层使用更小的接口，从而减少“因为一个表字段变更导致整串包一起改”的连锁反应。
- 它们也让 `pkg/api` 能继续作为稳定的 HTTP / SSE / CLI wire model，而不是被内部存储细节拖着走。
- 它们把 `spec03` 的四块骨架直接落在仓库里，维护者不需要再靠口头上下文判断一个模块到底属于哪一层。

## Verification

- 四块 owner map: `tests/architecture/four_plane_boundaries_test.go`
- 架构测试: `tests/architecture/boundaries_test.go`
- Runtime 边界: `tests/architecture/runtime_kernel_boundaries_test.go`
- owner closure: `tests/architecture/owner_closure_test.go`
- 应用装配: `internal/bootstrap/app.go`
- HTTP 路由: `internal/http/server.go`
- CLI 根命令: `cmd/simiclaw/internal/root/command.go`

## Related Docs

- 系统总览: [`../../ARCHITECTURE.md`](../../ARCHITECTURE.md)
- 四块骨架: [`four-plane-architecture.md`](four-plane-architecture.md)
- 运行链路: [`runtime-flow.md`](runtime-flow.md)
- 配置参考: [`../references/configuration.md`](../references/configuration.md)
- 测试矩阵: [`../references/testing.md`](../references/testing.md)
