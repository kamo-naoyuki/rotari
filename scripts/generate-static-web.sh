#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_dir=$(cd "${script_dir}/.." && pwd)
output_dir=${1:-"${repo_dir}/docs/web-demo"}
# DEMO_WORK_DIR keeps the binary, state, and workspace there for reuse, as
# scripts/web-structure.sh does; otherwise they are removed on exit.
if [[ -n "${DEMO_WORK_DIR:-}" ]]; then
    work_dir=${DEMO_WORK_DIR}
    mkdir -p "${work_dir}"
else
    work_dir=$(mktemp -d)
    trap 'rm -rf "${work_dir}"' EXIT INT TERM
fi

binary="${work_dir}/rotari"
state_dir="${work_dir}/state"
go_binary=${GO_BINARY:-/usr/bin/go}

if [[ ! -x "${go_binary}" ]]; then
    echo "Go compiler not found at ${go_binary}; set GO_BINARY to its absolute path" >&2
    exit 1
fi

echo "building rotari..."
(cd "${repo_dir}" && "${go_binary}" build -o "${binary}" ./cmd/rotari)

export ROTARI_BASEDIR="${state_dir}"
export ROTARI_PROJECT_NAME=demo

"${binary}" add --job-name prepare sh -c 'echo preparation complete'
"${binary}" add --job-name train --depends-on prepare sh -c 'echo training complete'
"${binary}" add --job-name failed sh -c 'echo validation failed; exit 1'
"${binary}" run --run-name "Demo run" || true
first_run_id=$(find "${state_dir}/projects/demo/runs" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | sort | tail -n 1)
if [[ -z "${first_run_id}" ]]; then
    echo "first run was not created" >&2
    exit 1
fi

# Fix the failing job, then retry only it. "prepare" and "train" already
# succeeded, so they are carried forward from the Demo run instead of being
# re-executed: the retry run's page links back to their original output.
# Restore the failed job into the current queue before editing it.
"${binary}" copy --run-id "${first_run_id}" --failed --overwrite
"${binary}" change --job-name failed -- sh -c 'echo validation now passes'
"${binary}" run --failed --run-name "Retry run" \
    --note "Retry only the failed validation job after fixing its check."

# Keep a useful current queue in the demo so the queue page is not empty.
run_count=$(find "${state_dir}/projects/demo/runs" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | wc -l)
if [[ "${run_count}" -lt 2 ]]; then
    echo "expected two demo runs, found ${run_count}" >&2
    exit 1
fi
"${binary}" copy --run-id "${first_run_id}" --failed --unfinished --overwrite

# A parameter sweep for the run page's matrix grid. With three dimensions the
# page shows one LR x SEED grid per MODEL: LR=0.001 fails everywhere,
# LR=0.1 fails for every seed of the large model, and one small-model seed
# fails on its own. "collect" still runs because it only needs the sweep to
# finish.
"${binary}" add -p sweep --job-name train \
    --matrix LR=0.1,0.01,0.001 --matrix SEED=1,2,3 --matrix MODEL=small,large \
    sh -c 'echo "lr=$LR seed=$SEED model=$MODEL"
		case "$MODEL/$LR/$SEED" in
		*/0.001/* | large/0.1/* | small/0.1/3) exit 1 ;;
		esac'
"${binary}" add -p sweep --job-name collect --depends-on-finished train \
    sh -c 'echo collected the finished sweep results'
"${binary}" run -p sweep --run-name "Hyperparameter sweep" || true

# An array whose tasks fail with different exit codes, so array task views
# show more than one kind of failure.
"${binary}" add -p shards --job-name decode --array 1-6 \
    sh -c 'echo "decoding shard $ROTARI_ARRAY_TASK_ID"
		case "$ROTARI_ARRAY_TASK_ID" in
		3) exit 2 ;;
		5) exit 137 ;;
		esac'
"${binary}" run -p shards --run-name "Decode shards" || true

# A training job whose files cover every artifact view: images, audio,
# video, tables, logs, text, JSON, NumPy arrays, an opaque checkpoint, a
# directory, and a missing file. Its candidates come from every kind of
# source: the script it runs and what that script names, the --config file
# and the paths inside it, a Python file's argparse default, an --env value,
# and its --output log. The files live in the temporary work directory; the
# export records what was there when it ran.
workspace="${work_dir}/workspace"
mkdir -p "${workspace}/conf"
python3 "${script_dir}/demo_artifacts.py" "${workspace}"
cat >"${workspace}/conf/train.yaml" <<'YAML'
data_path: data/train.csv
output_dir: results
epochs: 3
YAML
cat >"${workspace}/evaluate.py" <<'PYTHON'
import argparse

parser = argparse.ArgumentParser()
parser.add_argument("--report-path", default="results/summary.json")
parser.parse_args()
print("evaluation complete")
PYTHON
cat >"${workspace}/train.sh" <<'SCRIPT'
echo "epoch 3: loss 0.41" >> results/train.log
cat results/metrics.csv results/predictions.tsv results/notes.txt > /dev/null
ls results/plot.png results/loss.svg results/tone.wav results/clip.mp4 > /dev/null
ls results/weights.npy results/arrays.npz results/model.pt results/checkpoints/ > /dev/null
test -f results/eval.csv || echo "no evaluation table yet"
python3 evaluate.py
SCRIPT
"${binary}" add -p artifacts --job-name train --working-directory "${workspace}" \
    --env CACHE_DIR=results/checkpoints --output logs/train.log \
    -- sh train.sh --config conf/train.yaml
"${binary}" run -p artifacts --run-name "Training with artifacts"

echo "generating static pages in ${output_dir}..."
# Export every project, not only the one ROTARI_PROJECT_NAME selects.
env -u ROTARI_PROJECT_NAME "${binary}" web --static-dir "${output_dir}" --static-artifact-contents
echo "done"
