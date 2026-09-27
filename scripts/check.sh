#!/usr/bin/env bash
# Run the checks from CI's go job in one command.
#
#   scripts/check.sh           go vet, go test, and go test -race
#   scripts/check.sh --short   go vet and go test -short, for the edit loop
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

if [[ "$short" == true ]]; then
    step "go test -short"
    go test -short ./...
else
    step "go test"
    go test ./...
    step "go test -race"
    go test -race ./...
fi

step "all checks passed"
