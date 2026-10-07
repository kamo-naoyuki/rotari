#!/usr/bin/env bash
set -eu

export ROTARI_PROJECT_NAME=dependencies-example

# train waits for prepare to finish successfully.
rotari reset
rotari add --job-name prepare -- echo preparing inputs
rotari add --job-name train --depends-on prepare -- echo training
rotari run
rotari show

echo "Inspect this project later with: rotari show -p $ROTARI_PROJECT_NAME"
