#!/usr/bin/env bash
set -eu

export ROTARI_PROJECT_NAME=async-example

rotari reset
rotari add --job-name background -- sh -c 'sleep 1; echo finished'
rotari run --async
rotari wait "$ROTARI_PROJECT_NAME"
rotari show

echo "Inspect this project later with: rotari show -p $ROTARI_PROJECT_NAME"
