#!/usr/bin/env bash
set -eu

export ROTARI_PROJECT_NAME=basic-example

# Clear the next-run queue; completed run history is kept.
rotari reset

# A normal command, then a shell command using sh's -c option.
rotari add echo "hello from rotari"
rotari add sh -c 'echo "hello from a shell"'

# A command name starting with - needs -- so it is not parsed as a rotari
# option.
command_dir=$(mktemp -d)
trap 'rm -rf "$command_dir"' EXIT
printf '%s\n' '#!/bin/sh' 'echo "command beginning with a hyphen"' > \
	"$command_dir/-rotari-demo"
chmod +x "$command_dir/-rotari-demo"
export PATH="$command_dir:$PATH"
rotari add -- -rotari-demo

# Run the queued jobs and wait for them to finish.
rotari run

# Show the latest run (or the queue if no run has completed).
rotari show

echo "Inspect this project later with: rotari show -p $ROTARI_PROJECT_NAME"
