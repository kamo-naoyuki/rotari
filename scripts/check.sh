#!/usr/bin/env bash
# Run the checks from CI's go and web-format jobs in one command.
#
#   scripts/check.sh           gofmt, web asset formatting, go vet, go test,
#                              and go test -race
#   scripts/check.sh --short   gofmt, go vet, and go test -short, for the
#                              edit loop
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

step gofmt
# Tracked files only: local caches such as .gomodcache hold Go sources too.
unformatted=$(git ls-files -z '*.go' | xargs -0 gofmt -l)
if [[ -n "$unformatted" ]]; then
    echo "gofmt needed:" >&2
    echo "$unformatted" >&2
    exit 1
fi

if [[ "$short" == false ]]; then
    step "web asset formatting"
    if [[ -x node_modules/.bin/prettier ]]; then
        npm run --silent format:check
    else
        echo "skipped: run npm ci to install prettier" >&2
    fi
fi

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
