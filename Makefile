SHELL := /bin/bash

CORE_PKGS := ./internal/gateway/... ./internal/runtime/... ./internal/session/... ./internal/store/...
GUARDRAILS_BASELINE := .github/guardrails/baseline.json
GUARDRAILS_ALLOWLIST := .github/guardrails/allowlist.yaml
GUARDRAILS_REPORT ?= /tmp/simiclaw-guardrails-report.json
MARKDOWN_TARGETS := "./*.md" "./docs/**/*.md"

.PHONY: fmt fmt-check vet lint lint-ci test-architecture test-unit test-unit-race-core test-integration test-e2e-smoke test-e2e accept-v1 accept-v1-alpha accept-current web-ci docs-style docs-links guardrails-check guardrails-report guardrails-baseline-refresh

fmt:
	@find . -name '*.go' -not -path './.git/*' -print0 | xargs -0 gofmt -w

fmt-check:
	@files=$$(git ls-files '*.go' | while read -r file; do \
		[[ "$$file" == *.pb.go || "$$file" == *_gen.go || "$$file" == mocks/* || "$$file" == */mocks/* ]] && continue; \
		if grep -q "Code generated" "$$file" 2>/dev/null && grep -q "DO NOT EDIT" "$$file" 2>/dev/null; then \
			continue; \
		fi; \
		printf '%s\n' "$$file"; \
	done); \
	if [[ -z "$$files" ]]; then \
		exit 0; \
	fi; \
	unformatted=$$(printf '%s\n' "$$files" | xargs gofmt -l); \
	if [[ -n "$$unformatted" ]]; then \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet:
	go vet ./...

lint:
	$(MAKE) lint-ci

lint-ci:
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint is required for lint-ci"; \
		exit 1; \
	fi
	golangci-lint run --config .golangci.yml ./...

test-architecture:
	go test ./tests/architecture/... -v

test-unit:
	@if go tool | grep -qx 'covdata'; then \
		go test ./cmd/... ./internal/... ./pkg/... ./tools/... -coverprofile=/tmp/simiclaw-unit.cover; \
		go tool cover -func=/tmp/simiclaw-unit.cover | tail -n 1; \
	else \
		echo "covdata not available, running unit tests without coverage profile"; \
		go test ./cmd/... ./internal/... ./pkg/... ./tools/...; \
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

web-ci:
	npm ci --prefix web
	npm run build --prefix web
	npm run test --prefix web

docs-style:
	@if command -v npx >/dev/null 2>&1; then \
		npx --yes markdownlint-cli2 --config .markdownlint-cli2.jsonc $(MARKDOWN_TARGETS); \
	else \
		echo "npx is required for docs-style"; \
		exit 1; \
	fi

docs-links:
	@if ! command -v lychee >/dev/null 2>&1; then \
		echo "lychee is required for docs-links"; \
		exit 1; \
	fi
	lychee --config .lychee.toml $(MARKDOWN_TARGETS)

guardrails-check:
	@if [[ -n "$$GUARDRAILS_BASE" && -n "$$GUARDRAILS_HEAD" ]]; then \
		go run ./tools/ci-guardrails check --scope pr --base "$$GUARDRAILS_BASE" --head "$$GUARDRAILS_HEAD" --baseline "$(GUARDRAILS_BASELINE)" --allowlist "$(GUARDRAILS_ALLOWLIST)"; \
	else \
		go run ./tools/ci-guardrails check --scope repo --baseline "$(GUARDRAILS_BASELINE)" --allowlist "$(GUARDRAILS_ALLOWLIST)"; \
	fi

guardrails-report:
	go run ./tools/ci-guardrails check --scope repo --baseline "$(GUARDRAILS_BASELINE)" --allowlist "$(GUARDRAILS_ALLOWLIST)" --json "$(GUARDRAILS_REPORT)"

guardrails-baseline-refresh:
	$(MAKE) guardrails-report
	go run ./tools/ci-guardrails baseline sync --report "$(GUARDRAILS_REPORT)" --baseline "$(GUARDRAILS_BASELINE)"

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
