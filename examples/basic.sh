#!/usr/bin/env bash
set -eu

export ROTARI_BASEDIR="$PWD/.example-state"
export ROTARI_PROJECT_NAME=basic-example

# A dependent job starts only after prepare succeeds.
rotari reset --recover
rotari add --job-name prepare -- sh -c 'echo preparing inputs'
rotari add --job-name train --depends-on prepare -- sh -c 'echo training'
rotari run
rotari show

echo "Example state: $ROTARI_BASEDIR"
