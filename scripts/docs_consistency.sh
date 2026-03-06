#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if command -v rg >/dev/null 2>&1; then
  has_pattern() {
    rg -q -- "$1" "${@:2}"
  }
else
  has_pattern() {
    grep -Eq -- "$1" "${@:2}"
  }
fi

if ! has_pattern '^V1_ALPHA$' VERSION_STAGE; then
  echo "VERSION_STAGE must be V1_ALPHA"
  exit 1
fi

if ! has_pattern 'v1\.0 alpha' README.md; then
  echo "README missing v1.0 alpha marker"
  exit 1
fi

required_targets=(
  "make fmt"
  "make vet"
  "make lint"
  "make test-unit"
  "make test-unit-race-core"
  "make test-integration"
  "make test-e2e-smoke"
  "make accept-v1-alpha"
  "make accept-current"
)

for target in "${required_targets[@]}"; do
  if ! has_pattern "$target" README.md; then
    echo "README missing target: $target"
    exit 1
  fi
done

if ! has_pattern '^accept-v1-alpha:' Makefile; then
  echo "Makefile missing accept-v1-alpha target"
  exit 1
fi

if has_pattern 'accept-m[1-4]' Makefile README.md; then
  echo "legacy acceptance targets still referenced"
  exit 1
fi

echo "docs consistency check passed"
