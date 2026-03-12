# Testing Reference

## Summary

SimiClaw 的测试分成 architecture、unit、integration、e2e 和按阶段聚合的 acceptance。命令权威来源是仓库根部 `Makefile`，目录权威来源是 `tests/` 与各包内的 `_test.go`。

## 测试矩阵

| 范围 | 命令 | 说明 |
| --- | --- | --- |
| 格式化 | `make fmt` | 对全部 Go 文件运行 `gofmt -w` |
| 静态分析 | `make vet` | `go vet ./...` |
| Lint | `make lint` | 优先 `golangci-lint run ./...`，缺失时回退 `go vet` |
| 单元测试 | `make test-unit` | 跑 `./cmd/... ./internal/... ./pkg/...`，尽可能带 coverage |
| 核心 race | `make test-unit-race-core` | 只对 `gateway/runtime/session/store` 跑 `-race` |
| 集成测试 | `make test-integration` | `./tests/integration/...`，使用 `integration` build tag |
| E2E smoke | `make test-e2e-smoke` | 根据 `VERSION_STAGE` 选择 `SmokeV1` 或 `SmokeV1Alpha` |
| 全量 E2E | `make test-e2e` | `./tests/e2e/... -count=1` |
| 当前阶段验收 | `make accept-current` | 根据 `VERSION_STAGE` 选择对应 acceptance 组合 |

## 目录含义

- `tests/architecture/`: 架构边界与依赖方向的保护测试
- `tests/integration/`: 进程内集成测试，覆盖 runtime 生命周期与 Telegram 集成
- `tests/e2e/`: 面向阶段 smoke 的端到端验证
- 各 `internal/` / `cmd/` / `pkg/` 包内 `_test.go`: 单元与局部行为测试

## 单测与调试示例

```bash
go test ./internal/session/... -run TestComputeKeyDMThreadIgnored -v
go test ./internal/config/... -run TestLoad -v
go test ./tests/integration/... -tags=integration -run TestRuntimeSQLiteLifecycle -v
go test ./tests/e2e/... -run SmokeV1 -v -count=1
go test ./tests/architecture/... -v
```

## 阶段与验收

- `VERSION_STAGE` 当前为 `V1`
- `make test-e2e-smoke` 和 `make accept-current` 都会读取这个文件决定跑哪一套 smoke / acceptance
- 对文档和架构层改动，最小建议是至少跑 `go test ./tests/architecture/... -v`

## Verification

- 测试命令: `Makefile`
- 阶段定义: `VERSION_STAGE`
- 架构测试: `tests/architecture/boundaries_test.go`
- 集成测试: `tests/integration/runtime_integration_test.go`, `tests/integration/telegram_integration_test.go`
- E2E 测试: `tests/e2e/smoke_v1_test.go`

## Related Docs

- 配置参考: [`configuration.md`](configuration.md)
- 系统总览: [`../../ARCHITECTURE.md`](../../ARCHITECTURE.md)
- 运行链路: [`../design-docs/runtime-flow.md`](../design-docs/runtime-flow.md)
