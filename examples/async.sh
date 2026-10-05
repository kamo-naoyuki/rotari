#!/usr/bin/env bash
set -eu

export ROTARI_BASEDIR="$PWD/.example-state"
export ROTARI_PROJECT_NAME=async-example

rotari unlock
rotari reset
rotari add --job-name background -- sh -c 'sleep 1; echo finished'
rotari run --async
rotari wait "$ROTARI_PROJECT_NAME"
rotari show

echo "Example state: $ROTARI_BASEDIR"
