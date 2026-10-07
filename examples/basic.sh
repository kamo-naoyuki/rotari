#!/usr/bin/env bash
set -eu

export ROTARI_PROJECT_NAME=basic-example

rotari reset
rotari add echo "hello from rotari"
rotari add sh -c 'echo "hello from a shell"'
rotari run
rotari show

echo "Inspect this project later with: rotari show -p $ROTARI_PROJECT_NAME"
