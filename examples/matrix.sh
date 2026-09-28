#!/usr/bin/env bash
set -eu

export ROTARI_BASEDIR="$PWD/.example-state"
export ROTARI_PROJECT_NAME=matrix-example

# Two axes create four independent jobs; each receives SEED and MODEL.
rotari reset --recover
rotari add --job-name train --matrix SEED=1,2 --matrix MODEL=small,large \
    -- sh -c 'echo "training MODEL=$MODEL SEED=$SEED"'
rotari run --local-concurrency 2
rotari show --run-id latest

echo "Example state: $ROTARI_BASEDIR"