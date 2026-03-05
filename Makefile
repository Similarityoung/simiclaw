SHELL := /bin/bash

CORE_PKGS := ./pkg/gateway/... ./pkg/runtime/... ./pkg/store/... ./pkg/idempotency/... ./pkg/sessionkey/...

.PHONY: fmt vet lint test-unit test-unit-race-core test-integration test-e2e-smoke test-e2e test-fault-injection test-patch-guard accept-current accept-m1 accept-m2 accept-m3 accept-m4 docs-consistency

# 格式化所有 Go 代码
fmt:
	@find . -name '*.go' -not -path './.git/*' -print0 | xargs -0 gofmt -w

# 运行基础静态语法检查
vet:
	go vet ./...

# 运行 linter 进行代码规范检查（如果未安装 golangci-lint 则回退到 go vet）
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, fallback to go vet"; \
		go vet ./...; \
	fi

# 运行所有单元测试并输出测试覆盖率
test-unit:
	@if go tool | grep -qx 'covdata'; then \
		go test ./cmd/... ./pkg/... -coverprofile=/tmp/simiclaw-unit.cover; \
		go tool cover -func=/tmp/simiclaw-unit.cover | tail -n 1; \
	else \
		echo "covdata not available, running unit tests without coverage profile"; \
		go test ./cmd/... ./pkg/...; \
	fi

# 针对核心逻辑包运行带有数据竞争检测 (-race) 的单元测试
test-unit-race-core:
	@missing=0; \
	for pkg in $(CORE_PKGS); do \
		if go list $$pkg >/dev/null 2>&1; then \
			echo "running race test on $$pkg"; \
			go test -race $$pkg || exit 1; \
		else \
			echo "core package pending: $$pkg"; \
			missing=1; \
		fi; \
	done; \
	if [[ $$missing -eq 1 ]]; then echo "core package pending"; fi

# 运行集成测试，验证模块间交互（需带 -tags=integration）
test-integration:
	go test ./tests/integration/... -tags=integration

# 运行端到端 (E2E) 冒烟测试，仅验证核心链路连通性
test-e2e-smoke:
	@stage=$$(cat VERSION_STAGE); \
	echo "running e2e smoke for $$stage"; \
	case "$$stage" in \
		M1) regex='SmokeM1' ;; \
		M2) regex='SmokeM1|SmokeM2' ;; \
		M3) regex='SmokeM1|SmokeM2|SmokeM3' ;; \
		M4) regex='SmokeM1|SmokeM2|SmokeM3|SmokeM4' ;; \
		*) echo "unknown VERSION_STAGE=$$stage"; exit 1 ;; \
	esac; \
	go test ./tests/e2e/... -run "$$regex"

# 运行所有端到端 (E2E) 测试，禁止缓存
test-e2e:
	go test ./tests/e2e/... -count=1

# Patch Guard 最小回归入口（schema/lint/smoke 的 M4 护栏）
test-patch-guard:
	go test ./pkg/approval/... -run 'TestPatch|TestDecide' -v

# 运行故障注入测试（尚未实现）
test-fault-injection:
	@echo "stage not ready: test-fault-injection"

# 运行 M1 阶段的自动化验收测试
accept-m1: test-integration test-e2e-smoke
	@echo "accept-m1 passed"

# 运行 M2 阶段的自动化验收测试
accept-m2: test-integration test-e2e-smoke
	@echo "accept-m2 passed"

# 运行 M3 阶段的自动化验收测试
accept-m3: test-integration test-e2e-smoke
	@echo "accept-m3 passed"

# 运行 M4 阶段的自动化验收测试
accept-m4: test-integration test-e2e-smoke test-patch-guard
	@echo "accept-m4 passed"

# 根据 VERSION_STAGE 自动选择执行当前开发阶段的验收流程
accept-current:
	@stage=$$(cat VERSION_STAGE); \
	case "$$stage" in \
		M1) $(MAKE) accept-m1 ;; \
		M2) $(MAKE) accept-m2 ;; \
		M3) $(MAKE) accept-m3 ;; \
		M4) $(MAKE) accept-m4 ;; \
		*) echo "unknown VERSION_STAGE=$$stage"; exit 1 ;; \
	esac

# 检查项目文档的一致性
docs-consistency:
	./scripts/docs_consistency.sh
