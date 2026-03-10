SHELL := /bin/bash

CORE_PKGS := ./internal/gateway/... ./internal/runtime/... ./internal/session/... ./internal/store/...

.PHONY: fmt vet lint test-unit test-unit-race-core test-integration test-e2e-smoke test-e2e accept-v1 accept-v1-alpha accept-current

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
	@if go tool | grep -qx 'covdata'; then \
		go test ./cmd/... ./internal/... ./pkg/... -coverprofile=/tmp/simiclaw-unit.cover; \
		go tool cover -func=/tmp/simiclaw-unit.cover | tail -n 1; \
	else \
		echo "covdata not available, running unit tests without coverage profile"; \
		go test ./cmd/... ./internal/... ./pkg/...; \
	fi

test-unit-race-core:
	@for pkg in $(CORE_PKGS); do \
		echo "running race test on $$pkg"; \
		go test -race $$pkg || exit 1; \
	done

test-integration:
	go test ./tests/integration/... -tags=integration

test-e2e-smoke:
	@stage=$$(cat VERSION_STAGE); \
	if [[ "$$stage" == "V1" ]]; then \
		go test ./tests/e2e/... -run 'SmokeV1'; \
	elif [[ "$$stage" == "V1_ALPHA" ]]; then \
		go test ./tests/e2e/... -run 'SmokeV1Alpha'; \
	else \
		echo "unknown VERSION_STAGE=$$stage"; exit 1; \
	fi

test-e2e:
	go test ./tests/e2e/... -count=1

accept-v1: test-unit test-unit-race-core test-integration test-e2e-smoke
	@echo "accept-v1 passed"

accept-v1-alpha: accept-v1

accept-current:
	@stage=$$(cat VERSION_STAGE); \
	if [[ "$$stage" == "V1" ]]; then \
		$(MAKE) accept-v1; \
	elif [[ "$$stage" == "V1_ALPHA" ]]; then \
		$(MAKE) accept-v1-alpha; \
	else \
		echo "unknown VERSION_STAGE=$$stage"; exit 1; \
	fi
