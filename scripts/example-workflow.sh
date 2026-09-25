#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_dir=$(cd "${script_dir}/.." && pwd)
PATH="${repo_dir}:${PATH}"
export PATH

if (($# > 1)) || [[ "${1:-}" == -* ]]; then
    echo "usage: $0 [PROJECT_NAME]" >&2
    exit 2
fi

export ROTARI_BASEDIR=${ROTARI_BASEDIR:-"${repo_dir}/.rotari-state"}
export ROTARI_PROJECT_NAME=${ROTARI_PROJECT_NAME:-${1:-workflow-demo}}

manifest="${ROTARI_BASEDIR}/example-workflow.yaml"
edited="${ROTARI_BASEDIR}/example-workflow-rerun.yaml"
mkdir -p "${ROTARI_BASEDIR}"

# Discard any queue left over from a previous, possibly interrupted run of
# this script, so the import below starts from an empty queue.
rotari reset --recover

# The job graph lives in a reviewable file instead of a sequence of "add"
# commands. "evaluate" has a bug and "report" exits non-zero although its
# output is usable, so the first run has two failures to deal with.
cat >"${manifest}" <<'EOF'
version: 1

jobs:
  - name: prepare
    stage: inputs
    command: [sh, -c, "sleep 1; echo preparing inputs"]

  # Two independent jobs, train-SEED1 and train-SEED2, with SEED exported.
  - name: train
    command: [sh, -c, 'sleep 1; echo "training with SEED=${SEED}"']
    depends_on: [inputs]
    matrix: ["SEED=1,2"]

  - name: evaluate
    command: [sh, -c, "echo evaluating; exit 1"]
    depends_on: [train]

  - name: report
    command: [sh, -c, "echo report written with warnings; exit 3"]
    depends_on: [train]
EOF

rotari import --dry-run "${manifest}"
rotari import "${manifest}"

# evaluate and report fail, so the run reports a non-zero exit code;
# continue instead of aborting the script.
rotari run || true

# Export the finished run. The manifest records each job's status and source
# attempt, so an edited copy can be reconciled with this run.
rotari export --run-id latest >"${edited}"

# Edit the exported manifest the way a user would in an editor:
# - fix evaluate's command, which makes it new work;
# - after reviewing report's log, accept its failed result as successful.
sed -i.bak 's/echo evaluating; exit 1/echo evaluating; exit 0/' "${edited}"
awk '
    /^ *- name: / { job = $3 }
    job == "report" && /^ *status: failed$/ { sub(/failed/, "success") }
    { print }
' "${edited}" >"${edited}.tmp"
mv "${edited}.tmp" "${edited}"
rm -f "${edited}.bak"

# The plan shows prepare and both train jobs reused from the first run,
# evaluate executed, and report accepted, each with its source attempt.
rotari import --dry-run "${edited}"
rotari import "${edited}"

# Only evaluate executes. report is recorded as "success (accepted)" and
# still links to its original failed attempt.
rotari run
rotari show --run-id latest
