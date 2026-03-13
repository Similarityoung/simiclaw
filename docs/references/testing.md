# Testing Reference

## Summary

SimiClaw 的测试分成 architecture、unit、integration、e2e 和按阶段聚合的 acceptance。命令权威来源是仓库根部 `Makefile`，目录权威来源是 `tests/` 与各包内的 `_test.go`。

## 测试矩阵

| 范围 | 命令 | 说明 |
| --- | --- | --- |
| 格式化 | `make fmt` | 对全部 Go 文件运行 `gofmt -w` |
| 格式检查 | `make fmt-check` | 只检查 tracked、非 generated Go 文件是否已格式化 |
| 静态分析 | `make vet` | `go vet ./...` |
| Lint | `make lint-ci` | 运行固定配置的 `golangci-lint` |
| 架构测试 | `make test-architecture` | 跑 `./tests/architecture/...`，保护依赖方向和结构边界 |
| 单元测试 | `make test-unit` | 跑 `./cmd/... ./internal/... ./pkg/... ./tools/...`，尽可能带 coverage |
| 核心 race | `make test-unit-race-core` | 只对 `gateway/runtime/session/store` 跑 `-race` |
| 集成测试 | `make test-integration` | `./tests/integration/...`，使用 `integration` build tag |
| E2E smoke | `make test-e2e-smoke` | 根据 `VERSION_STAGE` 选择 `SmokeV1` 或 `SmokeV1Alpha` |
| 全量 E2E | `make test-e2e` | `./tests/e2e/... -count=1` |
| 当前阶段验收 | `make accept-current` | 根据 `VERSION_STAGE` 选择对应 acceptance 组合 |
| 前端 CI | `make web-ci` | 安装 `web/` 依赖并执行 build/test |
| 文档样式 | `make docs-style` | 跑仓库 Markdown 文档样式检查 |
| 链接检查 | `make docs-links` | 使用 lychee 跑严格链接检查 |
| Guardrails 检查 | `make guardrails-check` | 跑 repo 守则扫描，默认按 repo 范围比较 baseline |
| Guardrails 报告 | `make guardrails-report` | 生成 repo 守则 JSON 报告 |
| Guardrails baseline 刷新 | `make guardrails-baseline-refresh` | 根据最新报告重写 baseline，需显式提交审核 |

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
make docs-style
make guardrails-check
```

## 阶段与验收

- `VERSION_STAGE` 当前为 `V1`
- `make test-e2e-smoke` 和 `make accept-current` 都会读取这个文件决定跑哪一套 smoke / acceptance
- 对文档和架构层改动，最小建议是至少跑 `go test ./tests/architecture/... -v`
- 对 skill、docs、CI 配置改动，最小建议是再补 `make docs-style`
- Guardrails 与 baseline 变更前，先跑 `make guardrails-check` 或 `make guardrails-report`

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
