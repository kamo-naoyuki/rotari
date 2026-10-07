#!/usr/bin/env bash
set -eu

export ROTARI_PROJECT_NAME=matrix-example

# Two axes create four independent jobs; each receives SEED and MODEL.
rotari reset
rotari add --job-name train --matrix SEED=1,2 --matrix MODEL=small,large \
    -- sh -c 'echo "training MODEL=$MODEL SEED=$SEED"'
rotari run --local-concurrency 2
rotari show --run-id latest

echo "Inspect this project later with: rotari show -p $ROTARI_PROJECT_NAME"
