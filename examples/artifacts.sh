#!/usr/bin/env bash
set -eu

export ROTARI_PROJECT_NAME=artifacts-example
work="$PWD/.rotari-example-work/artifacts"
mkdir -p "$work"

# --config and out_dir demonstrate paths discovered from a command argument
# and a config file; the shell redirection adds an output candidate.
# Keep -- before sh so Rotari does not treat the job's --config as its own.
rotari reset
printf 'out_dir: results\nlr: 0.1\n' > "$work/config.yaml"
rotari add --job-name train --working-directory "$work" -- sh -c '
	mkdir -p results
	printf "epoch,loss\n1,0.9\n2,0.5\n" > results/metrics.csv
	echo training' sh --config config.yaml

# An evaluation job writes short WAV files (square waves, made with printf
# so the example needs nothing but sh) and a table that names them by
# absolute path; --artifact declares the table, which is built at run time.
cat > "$work/evaluate.sh" <<'EOF'
# le writes an integer as $2 little-endian bytes.
le() {
	value=$1 count=$2
	while [ "$count" -gt 0 ]; do
		printf "\\$(printf %03o $((value % 256)))"
		value=$((value / 256)) count=$((count - 1))
	done
}
# wav writes $1: 8 kHz mono 16-bit, a square wave with $2 samples per half
# period, $3 periods long.
wav() {
	bytes=$(($2 * 2 * 2 * $3))
	{
		printf 'RIFF'; le $((36 + bytes)) 4; printf 'WAVEfmt '; le 16 4
		le 1 2; le 1 2; le 8000 4; le 16000 4; le 2 2; le 16 2
		printf 'data'; le "$bytes" 4
		high=$(le 8000 2) low=$(le $((65536 - 8000)) 2) period='' i=0
		while [ "$i" -lt "$2" ]; do period="$period$high"; i=$((i + 1)); done
		i=0
		while [ "$i" -lt "$2" ]; do period="$period$low"; i=$((i + 1)); done
		i=0
		while [ "$i" -lt "$3" ]; do printf '%s' "$period"; i=$((i + 1)); done
	} > "$1"
}
mkdir -p results/audio
printf 'utt_id,audio,score,text\n' > results/eval.csv
for spec in 'u1 8 250 0.93 hello' 'u2 6 400 0.71 good morning' 'u3 10 300 0.88 see you tomorrow'; do
	set -- $spec
	id=$1 half=$2 periods=$3 score=$4
	shift 4
	wav "results/audio/$id.wav" "$half" "$periods"
	printf '%s,%s/results/audio/%s.wav,%s,%s\n' "$id" "$PWD" "$id" "$score" "$*" >> results/eval.csv
done
echo evaluated
EOF
rotari add --job-name evaluate --working-directory "$work" --artifact results/eval.csv -- sh evaluate.sh
rotari run
rotari show --job-name train --artifacts --no-pager

echo "Inspect artifacts later with:"
printf '  rotari show -p %s --job-name train --artifacts\n' "$ROTARI_PROJECT_NAME"
echo "Open the evaluation table, with its audio, in the Table viewer:"
printf '  rotari tableview %s\n' "$work/results/eval.csv"
echo "  (or rotari web: the evaluate job's Artifacts, Launch with..., Table viewer)"
