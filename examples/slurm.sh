#!/usr/bin/env bash
set -eu

export ROTARI_BASEDIR="$PWD/.example-state"
export ROTARI_PROJECT_NAME=slurm-example

# Requires a configured Slurm cluster and its submission commands.
rotari unlock
rotari reset
rotari add --job-name sweep --executor slurm --array 1-2 \
    --executor-option=--cpus-per-task=1 \
    -- sh -c 'echo "Slurm task $ROTARI_ARRAY_TASK_ID"'
rotari run
rotari show

echo "Example state: $ROTARI_BASEDIR"
