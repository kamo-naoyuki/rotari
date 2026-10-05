#!/usr/bin/env bash
set -eu

export ROTARI_BASEDIR="$PWD/.example-state"
export ROTARI_PROJECT_NAME=basic-example

# The jobs work in their own directory, so the files they write stay out of
# the directory you run this script from.
work="$ROTARI_BASEDIR/basic-work"
mkdir -p "$work"

# A dependent job starts only after prepare succeeds. Each run records the
# files its jobs name as artifact candidates: prepare's redirections, and
# train's --config file, the out_dir inside it, and its metrics file.
rotari unlock
rotari reset
rotari add --job-name prepare --working-directory "$work" -- sh -c '
	mkdir -p data
	printf "x,y\n1,2\n3,4\n" > data/train.csv
	printf "out_dir: results\nlr: 0.1\n" > config.yaml
	echo preparing inputs'
rotari add --job-name train --depends-on prepare --working-directory "$work" -- sh -c '
	mkdir -p results
	printf "epoch,loss\n1,0.9\n2,0.5\n" > results/metrics.csv
	echo training' sh --config config.yaml
rotari run
rotari show

# One job's details end with its artifact candidates.
rotari show --job-name train --no-pager

echo "Example state: $ROTARI_BASEDIR"
