#!/usr/bin/env bash
set -eu

export ROTARI_BASEDIR="$PWD/.example-state"
export ROTARI_PROJECT_NAME=workflow-reconcile-example

rotari reset --recover
rotari import "$(dirname "$0")/workflow-reconcile.yaml"
rotari run || true
rotari export --run-id latest > "$ROTARI_BASEDIR/exported.yaml"

# Fix evaluate and accept report's failed result after reviewing its log.
sed -e 's/echo evaluating; exit 1/echo evaluating; exit 0/' \
    -e '/    - name: report$/,/    - name: /s/^      status: failed$/      status: success/' \
    "$ROTARI_BASEDIR/exported.yaml" > "$ROTARI_BASEDIR/reconciled.yaml"

rotari import --dry-run "$ROTARI_BASEDIR/reconciled.yaml"
rotari import "$ROTARI_BASEDIR/reconciled.yaml"
rotari run --failed --unfinished
rotari show --run-id latest

echo "Example state: $ROTARI_BASEDIR"
