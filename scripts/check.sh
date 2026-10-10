#!/usr/bin/env bash
# Run the checks from CI's go job in one command.
#
#   scripts/check.sh           go vet, Web UI type check, go test, and go test -race
#   scripts/check.sh --short   go vet, Web UI type check, and go test -short
# Formatting is handled by pre-commit.
#
# CI also generates API docs, checks the generated Python CLI metadata, and
# runs the container and Python jobs; run those steps from
# .github/workflows/ci.yml when a change touches them.
set -euo pipefail

short=false
case "${1:-}" in
"") ;;
--short) short=true ;;
*)
    echo "usage: $0 [--short]" >&2
    exit 2
    ;;
esac

cd "$(dirname "${BASH_SOURCE[0]}")/.."

step() {
    echo "==> $*"
}

step "go vet"
go vet ./...

# The Web UI scripts marked // @ts-check, checked against their JSDoc types
# (internal/webui/tsconfig.json), as CI does after npm ci.
step "npm run typecheck"
npm run --silent typecheck

if [[ "$short" == true ]]; then
    step "go test -short"
    go test -short ./...
else
    step "go test"
    go test ./...
    step "go test -race"
    # pairedits executes thousands of CLI invocations and can exceed Go's
    # default ten-minute package timeout under the full race-suite load.
    go test -race -timeout 15m ./...
fi

step "all checks passed"
