#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_dir=$(cd "${script_dir}/.." && pwd)
PATH="${repo_dir}:${PATH}"
export PATH

diagnose_help=$(rotari diagnose --help 2>&1 || true)
if ! printf '%s\n' "${diagnose_help}" | grep -Fq 'Usage of diagnose:'; then
    echo "the rotari binary does not include 'diagnose'; run: go build -o rotari ./cmd/rotari" >&2
    exit 1
fi

: "${ROTARI_LLM_API_KEY:?set ROTARI_LLM_API_KEY before running this example}"
: "${ROTARI_LLM_MODEL:?set ROTARI_LLM_MODEL before running this example}"

basedir=${ROTARI_BASEDIR:-$(mktemp -d "${TMPDIR:-/tmp}/rotari-diagnose.XXXXXX")}
project=${ROTARI_PROJECT_NAME:-llm-diagnose-example}
export ROTARI_BASEDIR=${basedir}
export ROTARI_PROJECT_NAME=${project}

# This job fails predictably and leaves a useful traceback for the diagnosis.
rotari add --job-name missing-module python3 -c \
    'import definitely_missing_rotari_example_module'

# A failed job makes "rotari run" return non-zero; that is expected here.
rotari run || true

runs_dir="${ROTARI_BASEDIR}/projects/${ROTARI_PROJECT_NAME}/runs"
run_dir=""
for candidate in "${runs_dir}"/*; do
    if [ -d "${candidate}" ]; then
        run_dir=${candidate}
        break
    fi
done
if [ -z "${run_dir}" ]; then
    echo "failed to find the example run directory" >&2
    exit 1
fi

job_dir=""
for candidate in "${run_dir}"/*; do
    if [ -d "${candidate}" ] && [ -f "${candidate}/command.json" ]; then
        job_dir=${candidate}
        break
    fi
done
if [ -z "${job_dir}" ]; then
    echo "failed to find the example job directory" >&2
    exit 1
fi

run_id=$(basename "${run_dir}")
job_id=$(basename "${job_dir}")

echo ""
echo "Sending the generated failure log to the configured LLM endpoint..."
rotari diagnose --run-id "${run_id}" --job-id "${job_id}" --model "${ROTARI_LLM_MODEL}"

echo ""
echo "Example state remains at: ${ROTARI_BASEDIR}"
