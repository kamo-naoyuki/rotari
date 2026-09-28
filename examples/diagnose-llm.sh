#!/usr/bin/env bash
set -eu

: "${ROTARI_LLM_API_KEY:?set ROTARI_LLM_API_KEY before running this example}"
: "${ROTARI_LLM_MODEL:?set ROTARI_LLM_MODEL before running this example}"
export ROTARI_BASEDIR="$PWD/.example-state"
export ROTARI_PROJECT_NAME=diagnose-llm-example

rotari reset --recover
rotari add --job-name missing-module -- python3 -c 'import definitely_missing_rotari_example_module'
rotari run || true
rotari diagnose --job-name missing-module --model "$ROTARI_LLM_MODEL"

echo "Example state: $ROTARI_BASEDIR"