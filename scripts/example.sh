#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_dir=$(cd "${script_dir}/.." && pwd)
PATH="${repo_dir}:${PATH}"
export PATH

use_slurm=false
project_name=""
while (($# > 0)); do
    case "$1" in
        --slurm)
            use_slurm=true
            ;;
        --)
            shift
            if (($# != 1)); then
                echo "usage: $0 [--slurm] [PROJECT_NAME]" >&2
                exit 2
            fi
            project_name=$1
            ;;
        -*)
            echo "usage: $0 [--slurm] [PROJECT_NAME]" >&2
            exit 2
            ;;
        *)
            if [[ -n "${project_name}" ]]; then
                echo "usage: $0 [--slurm] [PROJECT_NAME]" >&2
                exit 2
            fi
            project_name=$1
            ;;
    esac
    shift
done

export ROTARI_BASEDIR=${ROTARI_BASEDIR:-"${repo_dir}/.rotari-state"}
export ROTARI_PROJECT_NAME=${ROTARI_PROJECT_NAME:-${project_name:-demo}}

array_executor=local
array_executor_options=()
if [[ "${use_slurm}" == true ]]; then
    array_executor=slurm
    array_executor_options=(--executor-option "--cpus-per-task=2")
fi

# Discard any queue left over from a previous, possibly interrupted run of
# this script, so the jobs added below never collide with earlier ones.
rotari reset --recover

# Stable markers outside any run directory: these jobs fail on their first
# attempt and succeed afterwards, so "rotari retry" below has a real
# failure to recover from.
rm -f "${ROTARI_BASEDIR}/example-array-task-1-marker" "${ROTARI_BASEDIR}/example-failing-job-marker"

# Mix local jobs in one queue. Pass --slurm to use Slurm for the array job.
# The two jobs after prepare can run in parallel with each other.
rotari add --job-name prepare --executor local sh -c 'sleep 2; echo preparation job'
rotari add --job-name array-job --depends-on prepare \
    --executor "${array_executor}" --array 1-2 "${array_executor_options[@]}" \
    sh -c 'sleep 2; echo "Array task ${ROTARI_ARRAY_TASK_ID}"
marker="${ROTARI_BASEDIR}/example-array-task-1-marker"
if [ "${ROTARI_ARRAY_TASK_ID}" = "1" ] && [ ! -f "${marker}" ]; then touch "${marker}"; exit 1; fi'
rotari add --job-name failing-job --depends-on prepare \
    --executor local \
    sh -c 'echo failing local job
marker="${ROTARI_BASEDIR}/example-failing-job-marker"
if [ ! -f "${marker}" ]; then touch "${marker}"; exit 1; fi'

# Run with separate local and batch executor concurrency limits. Array task
# 1 and failing-job fail on this first attempt, so the run reports a
# non-zero exit code; continue instead of aborting the script.
run_args=(--local-concurrency 2 --batch-concurrency 2)
rotari run "${run_args[@]}" || true

# By default, "rotari retry" (run --failed --unfinished) re-executes only
# failed/unfinished jobs, and for an array job only its failed tasks: task 1
# runs again here, while task 2 and "prepare" already succeeded and are
# carried forward from the first run instead of executing again. Both
# markers now exist, so every re-executed job succeeds this time.
rotari retry
