#!/usr/bin/env bash
set -eu

export ROTARI_PROJECT_NAME=artifacts-example
work="$PWD/.rotari-example-work/artifacts"
mkdir -p "$work"

# --config and out_dir demonstrate paths discovered from a command argument
# and a config file; the shell redirection adds an output candidate.
rotari reset
printf 'out_dir: results\nlr: 0.1\n' > "$work/config.yaml"
rotari add --job-name train --working-directory "$work" -- sh -c '
	mkdir -p results
	printf "epoch,loss\n1,0.9\n2,0.5\n" > results/metrics.csv
	echo training' sh --config config.yaml
rotari run
rotari show --job-name train --artifacts --no-pager

echo "Inspect artifacts later with:"
printf '  rotari show -p %s --job-name train --artifacts\n' "$ROTARI_PROJECT_NAME"
