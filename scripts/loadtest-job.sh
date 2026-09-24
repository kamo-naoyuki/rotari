#!/usr/bin/env bash
# Workload for scripts/loadtest.sh: sleeps a random 3-4s, then randomly fails.
set -uo pipefail

min=${ROTARI_LOADTEST_MIN_SLEEP:-3}
max=${ROTARI_LOADTEST_MAX_SLEEP:-4}
fail_percent=${ROTARI_LOADTEST_FAIL_PERCENT:-15}

read -r duration roll <<<"$(awk -v min="$min" -v max="$max" -v seed="$(( $$ ^ $(date +%N) ))" \
    'BEGIN { srand(seed); printf "%.2f %d", min + rand() * (max - min), int(rand() * 100) }')"

sleep "$duration"

if (( roll < fail_percent )); then
    echo "loadtest job failed (roll=${roll} < ${fail_percent}), slept ${duration}s" >&2
    exit 1
fi

echo "loadtest job succeeded (roll=${roll}, threshold=${fail_percent}), slept ${duration}s"
