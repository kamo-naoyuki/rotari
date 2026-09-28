#!/usr/bin/env bash
set -eu

export ROTARI_BASEDIR="$PWD/.example-state"
export ROTARI_PROJECT_NAME=diagnose-rules-example

rotari reset --recover
rotari add --job-name missing-module -- python3 -c 'import definitely_missing_rotari_example_module'
rotari run || true
rotari diagnose --job-name missing-module --rules

echo "Example state: $ROTARI_BASEDIR"