SHELL := /bin/bash

CORE_PKGS := ./pkg/gateway/... ./pkg/runtime/... ./pkg/store/... ./pkg/idempotency/... ./pkg/routing/...

.PHONY: fmt vet lint test-unit test-unit-race-core test-integration test-e2e-smoke test-e2e test-fault-injection accept-current accept-m1 accept-m2 accept-m3 accept-m4 docs-consistency

fmt:
	@find . -name '*.go' -not -path './.git/*' -print0 | xargs -0 gofmt -w

vet:
	go vet ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, fallback to go vet"; \
		go vet ./...; \
	fi

test-unit:
	go test ./cmd/... ./pkg/... -coverprofile=/tmp/simiclaw-unit.cover
	@go tool cover -func=/tmp/simiclaw-unit.cover | tail -n 1

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

test-integration:
	go test ./tests/integration/... -tags=integration

test-e2e-smoke:
	@stage=$$(cat VERSION_STAGE); \
	echo "running e2e smoke for $$stage"; \
	go test ./tests/e2e/... -run Smoke

test-e2e:
	go test ./tests/e2e/... -count=1

test-fault-injection:
	@echo "stage not ready: test-fault-injection"

accept-m1: test-integration test-e2e-smoke
	@echo "accept-m1 passed"

accept-m2:
	@echo "stage not ready: accept-m2 (M2 pending)"

accept-m3:
	@echo "stage not ready: accept-m3 (M3 pending)"

accept-m4:
	@echo "stage not ready: accept-m4 (M4 pending)"

accept-current:
	@stage=$$(cat VERSION_STAGE); \
	case "$$stage" in \
		M1) $(MAKE) accept-m1 ;; \
		M2) $(MAKE) accept-m2 ;; \
		M3) $(MAKE) accept-m3 ;; \
		M4) $(MAKE) accept-m4 ;; \
		*) echo "unknown VERSION_STAGE=$$stage"; exit 1 ;; \
	esac

docs-consistency:
	./scripts/docs_consistency.sh
