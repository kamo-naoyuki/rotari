#!/usr/bin/env bash
set -eu

export ROTARI_BASEDIR="$PWD/.example-state"
export ROTARI_PROJECT_NAME=array-example

# Each task receives its own ROTARI_ARRAY_TASK_ID.
rotari reset --recover
rotari add --job-name sweep --array 1-3 -- sh -c 'echo "task $ROTARI_ARRAY_TASK_ID"'
rotari run --local-concurrency 2
rotari show

echo "Example state: $ROTARI_BASEDIR"