#!/usr/bin/env bash
# Load test: submits many jobs split between the local and Slurm executors,
# with a few random dependencies among them, to exercise rotari under load.
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_dir=$(cd "${script_dir}/.." && pwd)
PATH="${repo_dir}:${PATH}"
export PATH

job_script="${script_dir}/loadtest-job.sh"

jobs=200
fail_percent=15
local_concurrency=20
batch_concurrency=20
dep_probability=40
max_deps=2
project_name=loadtest
slurm_options=()

usage() {
    cat >&2 <<'EOF'
usage: loadtest.sh [--jobs N] [--fail-percent P] [--local-concurrency N]
                    [--batch-concurrency N] [--dep-probability P]
                    [--slurm-option OPTION] [PROJECT_NAME]
EOF
}

while (($# > 0)); do
    case "$1" in
        --jobs) jobs=$2; shift 2 ;;
        --fail-percent) fail_percent=$2; shift 2 ;;
        --local-concurrency) local_concurrency=$2; shift 2 ;;
        --batch-concurrency) batch_concurrency=$2; shift 2 ;;
        --dep-probability) dep_probability=$2; shift 2 ;;
        --slurm-option) slurm_options+=("$2"); shift 2 ;;
        -h|--help) usage; exit 0 ;;
        --)
            shift
            if (($# != 1)); then
                usage
                exit 2
            fi
            project_name=$1
            shift
            ;;
        -*) usage; exit 2 ;;
        *)
            if (($# != 1)); then
                usage
                exit 2
            fi
            project_name=$1
            shift
            ;;
    esac
done

export ROTARI_BASEDIR=${ROTARI_BASEDIR:-"${repo_dir}/.rotari-state"}
export ROTARI_PROJECT_NAME=${ROTARI_PROJECT_NAME:-${project_name}}

# Discard any queue left over from a previous, possibly interrupted run.
rotari reset --recover

job_names=()
local_count=0
slurm_count=0
for ((i = 1; i <= jobs; i++)); do
    name=$(printf "loadtest-%04d" "$i")

    executor=local
    if (( i % 2 == 0 )); then
        executor=slurm
    fi

    deps=()
    dep_args=()
    if (( i > 1 )) && (( RANDOM % 100 < dep_probability )); then
        ndeps=$(( RANDOM % max_deps + 1 ))
        for ((d = 0; d < ndeps; d++)); do
            idx=$(( RANDOM % (i - 1) + 1 ))
            dep_name="${job_names[idx - 1]}"
            duplicate=0
            if (( ${#deps[@]} > 0 )); then
                for existing in "${deps[@]}"; do
                    if [[ "$existing" == "$dep_name" ]]; then
                        duplicate=1
                        break
                    fi
                done
            fi
            if (( duplicate == 0 )); then
                deps+=("$dep_name")
                dep_args+=(--depends-on "$dep_name")
            fi
        done
    fi

    executor_args=()
    if [[ "$executor" == slurm ]]; then
        for opt in "${slurm_options[@]:-}"; do
            [[ -n "$opt" ]] && executor_args+=(--executor-option "$opt")
        done
        slurm_count=$((slurm_count + 1))
    else
        local_count=$((local_count + 1))
    fi

    rotari add --job-name "$name" --executor "$executor" \
        --env "ROTARI_LOADTEST_FAIL_PERCENT=${fail_percent}" \
        "${dep_args[@]}" "${executor_args[@]}" -- bash "$job_script" \
        >/dev/null
    job_names+=("$name")
done

echo "submitted ${jobs} jobs (${local_count} local, ${slurm_count} slurm)"

rotari run --local-concurrency "$local_concurrency" --batch-concurrency "$batch_concurrency" || true

echo "loadtest finished; inspect with: rotari show -p ${ROTARI_PROJECT_NAME}"
