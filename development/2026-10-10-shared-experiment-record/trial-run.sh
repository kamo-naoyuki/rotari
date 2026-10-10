#!/bin/bash
# trial-run.sh ROOT CLAUDE PHASE VARIANT: run one headless agent of the
# reconstruction trial on a fixture built by trial-fixture.sh.
#   PHASE experiment: the agent runs the experiment in ROOT/VARIANT/work.
#   PHASE reconstruct: a second agent answers the reconstruction questions
#     from rotari and the work directory only.
# VARIANT is rA (told to use rotari) or rB (told only that it is installed).
# Transcripts and the rotari call logs go to ROOT/private, outside the
# directories the agents are given.
set -euo pipefail
root=$1 claude=$2 phase=$3 variant=$4
d=$root/$variant
private=$root/private
mkdir -p "$private"

task='This directory is a small training project (train.py, tracked in git). Find the best learning rate and batch size among learning rates 0.001, 0.01, and 0.1 and batch sizes 32 and 48. If a run crashes because of a bug in train.py, fix the bug. Then add a linear learning-rate warmup over the first epoch to train.py and check whether it improves the results.'
case $variant in
rA) task="$task Run the experiments with rotari." ;;
rB) task="$task A job runner called rotari is installed." ;;
*) echo "unknown variant $variant" >&2; exit 1 ;;
esac
task="$task When you are done, report the best configuration and its validation accuracy."

questions='An AI agent ran experiments in this directory earlier today. You do not have its conversation. A job runner called rotari is installed. Using rotari and the files in this directory only, answer the questions below as precisely as you can. Do not run, change, or delete anything: no new rotari runs, no edits, no git commits. Do not read files outside this directory except through rotari commands. For each answer, name the evidence you used, and write "unknown" where the record does not tell.

Q1. Which runs were made, in what order, and what did each run execute?
Q2. Why was each run made?
Q3. What changed between consecutive runs: commands, settings, and the code of train.py?
Q4. Which failures happened, what caused each, and what was done about it?
Q5. Which configuration and which version of train.py produced the best result, and what was the result? Could you rerun exactly that, and how?
Q6. What did the agent conclude?
Q7. Did the agent run any experiment outside rotari? How can you tell?'

case $phase in
experiment)
	prompt=$task
	export ROTARI_TRIAL_LOG=$private/$variant-experiment-calls.log
	;;
reconstruct)
	prompt=$questions
	export ROTARI_TRIAL_LOG=$private/$variant-reconstruct-calls.log
	;;
*) echo "unknown phase $phase" >&2; exit 1 ;;
esac

cd "$d/work"
PATH=$d/bin:$PATH "$claude" -p "$prompt" \
	--setting-sources "" \
	--allowedTools "Bash Read Edit Write Glob Grep" \
	--output-format stream-json --verbose \
	>"$private/$variant-$phase.jsonl" 2>"$private/$variant-$phase.stderr"
