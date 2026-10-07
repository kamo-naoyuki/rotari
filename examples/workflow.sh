#!/usr/bin/env bash
set -eu

export ROTARI_PROJECT_NAME=workflow-example
work="$PWD/.rotari-example-work/workflow"
mkdir -p "$work"

# The manifest groups two independent matrix jobs after the inputs stage.
rotari reset
rotari import --dry-run "$(dirname "$0")/workflow.yaml"
rotari import "$(dirname "$0")/workflow.yaml"
rotari run
rotari export --run-id latest > "$work/exported.yaml"

echo "Exported workflow: $work/exported.yaml"
