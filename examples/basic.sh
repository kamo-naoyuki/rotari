#!/usr/bin/env bash
set -eu

export ROTARI_PROJECT_NAME=basic-example

# Clear the next-run queue; completed run history is kept.
rotari reset

# A normal command, then a shell command using sh's -c option.
rotari add echo "hello from rotari"
rotari add sh -c 'echo "hello from a shell"'

# Run the queued jobs and wait for them to finish.
rotari run

# Show the latest run (or the queue if no run has completed).
rotari show

echo "Inspect this project later with: rotari show -p $ROTARI_PROJECT_NAME"
