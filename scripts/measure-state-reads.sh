#!/usr/bin/env bash
# Measure how many state files rotari's read commands touch, and how long
# they take, on the file system holding $TMPDIR (NFS, for the case that
# matters). Builds a fixture of finished runs under a fresh work directory,
# runs each command once untraced for wall time and once under strace for
# counts, prints a table, and removes the work directory.
#
#   scripts/measure-state-reads.sh [--projects N] [--runs N] [--tasks N] [--keep]
#
# Each project's first run executes an array of --tasks tasks and one job;
# every later run adds one job, carries the earlier results, and executes
# only the new job. See development/2026-10-15-nfs-read-performance/plan.md.
set -euo pipefail

projects=2
runs=15
tasks=20
keep=false

usage() {
    echo "usage: $0 [--projects N] [--runs N] [--tasks N] [--keep]" >&2
}

while (($# > 0)); do
    case "$1" in
    --projects) projects=$2 && shift 2 ;;
    --runs) runs=$2 && shift 2 ;;
    --tasks) tasks=$2 && shift 2 ;;
    --keep) keep=true && shift ;;
    *)
        usage
        exit 2
        ;;
    esac
done

command -v strace >/dev/null || {
    echo "strace is required" >&2
    exit 1
}

repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/rotari-reads.XXXXXX")
if ! $keep; then
    trap 'rm -rf "$work"' EXIT
fi
export XDG_STATE_HOME="$work/xdg" ROTARI_MASTERDIR="$work/master" ROTARI_BASEDIR="$work/base"
rotari="$work/rotari"
(cd "$repo_dir" && go build -o "$rotari" ./cmd/rotari)

for p in $(seq 1 "$projects"); do
    "$rotari" add -p "p$p" --job-name sweep --array "1-$tasks" -- true >/dev/null
    for r in $(seq 1 "$runs"); do
        "$rotari" add -p "p$p" --job-name "j$r" -- true >/dev/null
        "$rotari" run -p "p$p" --local-concurrency "$tasks" --quiet >/dev/null
    done
done
first_run=$(ls "$ROTARI_BASEDIR/projects/p1/runs" | head -1)

measure() {
    local start end trace="$work/trace"
    start=$(date +%s.%N)
    "$rotari" "$@" >/dev/null 2>&1 || true
    end=$(date +%s.%N)
    strace -f -qq -e trace=openat,newfstatat,statx,lstat,stat,getdents64 -o "$trace" "$rotari" "$@" >/dev/null 2>&1 || true
    printf '%-34s %8.3f %6d %6d %6d %6d\n' "$*" "$(echo "$end - $start" | bc)" \
        "$(grep -c 'openat(' "$trace" || true)" \
        "$(grep -c 'O_DIRECTORY' "$trace" || true)" \
        "$(grep -cE '(newfstatat|statx|lstat|stat)\(' "$trace" || true)" \
        "$(grep -c 'getdents64(' "$trace" || true)"
}

echo "fixture: $projects projects x $runs runs, $tasks-task array; work directory $work"
printf '%-34s %8s %6s %6s %6s %6s\n' command seconds open list stat dents
measure basedirs
measure projects
measure runs
measure runs --since 0
measure info -p p1
measure lineage -p p1
measure show -p p1
measure show -p p1 -r "$first_run"
measure jobs
measure jobs --since 0
