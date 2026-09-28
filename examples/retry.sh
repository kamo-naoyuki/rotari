#!/usr/bin/env bash
set -eu

export ROTARI_BASEDIR="$PWD/.example-state"
export ROTARI_PROJECT_NAME=retry-example

# This job fails once, then succeeds on the next run.
rotari reset --recover
rm -f "$ROTARI_BASEDIR/first-attempt"
rotari add --job-name flaky -- sh -c '
    test -f "$ROTARI_BASEDIR/first-attempt" || {
        touch "$ROTARI_BASEDIR/first-attempt"
        exit 1
    }
    echo recovered
'
rotari add --job-name stable -- sh -c 'echo already done'
rotari run || true
rotari retry
rotari show

echo "Example state: $ROTARI_BASEDIR"
