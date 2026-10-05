#!/usr/bin/env bash
set -eu

export ROTARI_BASEDIR="$PWD/.example-state"
export ROTARI_PROJECT_NAME=workflow-example

# The manifest groups two independent matrix jobs after the inputs stage.
rotari unlock
rotari reset
rotari import --dry-run "$(dirname "$0")/workflow.yaml"
rotari import "$(dirname "$0")/workflow.yaml"
rotari run
rotari export --run-id latest > "$ROTARI_BASEDIR/exported.yaml"

echo "Exported workflow: $ROTARI_BASEDIR/exported.yaml"
