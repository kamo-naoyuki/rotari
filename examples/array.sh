#!/usr/bin/env bash
set -eu

export ROTARI_PROJECT_NAME=array-example

# Each task receives its own ROTARI_ARRAY_TASK_ID.
rotari reset
rotari add --job-name sweep --array 1-3 sh -c 'echo "task $ROTARI_ARRAY_TASK_ID"'
rotari run --local-concurrency 2
rotari show

echo "Inspect this project later with: rotari show -p $ROTARI_PROJECT_NAME"
