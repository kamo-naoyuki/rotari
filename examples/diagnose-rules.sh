#!/usr/bin/env bash
set -eu

export ROTARI_PROJECT_NAME=diagnose-rules-example

rotari reset
rotari add --job-name missing-module python3 -c 'import definitely_missing_rotari_example_module'
rotari run || true
rotari show --job-name missing-module

echo "Inspect this project later with: rotari show -p $ROTARI_PROJECT_NAME"
