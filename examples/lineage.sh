#!/usr/bin/env bash
set -eu

export ROTARI_BASEDIR="$PWD/.example-state"
export ROTARI_PROJECT_NAME=lineage-example

# The first generation fails in train while eval succeeds.
rotari reset --recover
rotari add --job-name train -- false
rotari add --job-name eval -- true
rotari run || true

# Restore the failed run into the queue, fix the command, and run again. The
# successful eval result is reused.
rotari copy --run-id latest --overwrite
rotari change --job-name train -- true
rotari run --failed

# Compare the generations, or use a run ID to inspect one generation.
rotari lineage --basedir "$ROTARI_BASEDIR" --project-name "$ROTARI_PROJECT_NAME"

echo "Example state: $ROTARI_BASEDIR"
