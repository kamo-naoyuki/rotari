#!/usr/bin/env bash
set -eu

export ROTARI_PROJECT_NAME=retry-example
export ROTARI_EXAMPLE_WORK="$PWD/.rotari-example-work"
mkdir -p "$ROTARI_EXAMPLE_WORK"

# This job fails once, then succeeds on the next run.
rotari reset
rm -f "$ROTARI_EXAMPLE_WORK/retry-first-attempt"
rotari add --job-name flaky sh -c '
    test -f "$ROTARI_EXAMPLE_WORK/retry-first-attempt" || {
        touch "$ROTARI_EXAMPLE_WORK/retry-first-attempt"
        exit 1
    }
    echo recovered
'
rotari add --job-name stable sh -c 'echo already done'
rotari run || true
rotari retry
rotari show

echo "Inspect this project later with: rotari show -p $ROTARI_PROJECT_NAME"
