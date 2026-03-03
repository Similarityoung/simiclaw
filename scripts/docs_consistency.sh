#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

docs=(
  doc/requirements.md
  doc/architecture.md
  doc/interfaces.md
  doc/cicd-testing.md
  doc/roadmap.md
)

for f in "${docs[@]}"; do
  if ! rg -q "v0\\.4" "$f"; then
    echo "missing v0.4 marker: $f"
    exit 1
  fi
done

banned=(
  "postgres"
  "mysql"
  "数据库迁移"
)
for b in "${banned[@]}"; do
  if rg -n "$b" doc/*.md >/dev/null; then
    echo "banned term detected: $b"
    rg -n "$b" doc/*.md
    exit 1
  fi
done

targets=(
  "make fmt"
  "make vet"
  "make lint"
  "make test-unit"
  "make test-unit-race-core"
  "make test-integration"
  "make test-e2e-smoke"
  "make test-e2e"
  "make test-fault-injection"
  "make accept-current"
  "make accept-m1"
  "make accept-m2"
  "make accept-m3"
  "make accept-m4"
  "make docs-consistency"
)

for t in "${targets[@]}"; do
  if ! rg -q "$t" doc/roadmap.md; then
    echo "roadmap missing target: $t"
    exit 1
  fi
  if ! rg -q "$t" doc/cicd-testing.md; then
    echo "cicd-testing missing target: $t"
    exit 1
  fi
done

echo "docs consistency check passed"
