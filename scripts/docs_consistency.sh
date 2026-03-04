#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if command -v rg >/dev/null 2>&1; then
  has_pattern() {
    rg -q -- "$1" "${@:2}"
  }
  list_pattern() {
    rg -n -- "$1" "${@:2}"
  }
else
  has_pattern() {
    grep -Eq -- "$1" "${@:2}"
  }
  list_pattern() {
    grep -En -- "$1" "${@:2}"
  }
fi

docs=(
  doc/requirements.md
  doc/architecture.md
  doc/interfaces.md
  doc/cicd-testing.md
  doc/roadmap.md
)

for f in "${docs[@]}"; do
  if ! has_pattern "v0\\.4" "$f"; then
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
  if has_pattern "$b" doc/*.md; then
    echo "banned term detected: $b"
    list_pattern "$b" doc/*.md
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
  if ! has_pattern "$t" doc/roadmap.md; then
    echo "roadmap missing target: $t"
    exit 1
  fi
  if ! has_pattern "$t" doc/cicd-testing.md; then
    echo "cicd-testing missing target: $t"
    exit 1
  fi
done

echo "docs consistency check passed"
