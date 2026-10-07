#!/usr/bin/env bash
set -eu

export ROTARI_PROJECT_NAME=workflow-reconcile-example
work="$PWD/.rotari-example-work/workflow-reconcile"
mkdir -p "$work"

rotari reset
rotari import "$(dirname "$0")/workflow-reconcile.yaml"
rotari run || true
rotari export --run-id latest > "$work/exported.yaml"

# Fix evaluate and accept report's failed result after reviewing its log.
sed -e 's/echo evaluating; exit 1/echo evaluating; exit 0/' \
    -e '/^  - name: report$/,/^  - name: /s/^    status: failed$/    status: success/' \
    "$work/exported.yaml" > "$work/reconciled.yaml"

rotari import --dry-run "$work/reconciled.yaml"
rotari import "$work/reconciled.yaml"
rotari run --failed --unfinished
rotari show --run-id latest

echo "Inspect this project later with: rotari show -p $ROTARI_PROJECT_NAME"
