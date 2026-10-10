#!/bin/bash
# trial-fixture.sh ROOT REPO: build the reconstruction trial fixtures under
# ROOT. ROOT/rA and ROOT/rB each hold an isolated rotari (bin/rotari logs its
# calls to logs/calls.log), a git-tracked toy training project in work/, and
# empty state and config directories. rA's agent is told to use rotari; rB's
# only that it is installed. See plan.md, Phase 1.
set -euo pipefail
root=$1 repo=$2
rm -rf "$root/rA" "$root/rB" "$root/real"
mkdir -p "$root/real"
(cd "$repo" && go build -o "$root/real/rotari" ./cmd/rotari)

mkenv() { # mkenv DIR
	local d=$1
	mkdir -p "$d/bin" "$d/xdg" "$d/config" "$d/logs" "$d/work"
	cat >"$d/bin/rotari" <<W
#!/bin/bash
out=\$(mktemp); err=\$(mktemp)
export XDG_STATE_HOME=$d/xdg XDG_CONFIG_HOME=$d/config
"$root/real/rotari" "\$@" >"\$out" 2>"\$err"; rc=\$?
cat "\$out"; cat "\$err" >&2
printf '%s\trc=%s\tout=%s\terr=%s\trotari %s\n' "\$(date +%T)" \$rc \$(wc -c <"\$out") \$(wc -c <"\$err") "\$*" >>"\${ROTARI_TRIAL_LOG:-$d/logs/calls.log}"
rm -f "\$out" "\$err"; exit \$rc
W
	chmod +x "$d/bin/rotari"

	cat >"$d/work/train.py" <<'PY'
#!/usr/bin/env python3
"""Train a toy regression model and report its validation accuracy."""
import argparse
import math
import sys
import time

DATASET_SIZE = 4000


def load_dataset():
    return [((i % 97) / 97.0, ((i * 7) % 89) / 89.0) for i in range(DATASET_SIZE)]


def batches(data, batch_size):
    for start in range(0, len(data), batch_size):
        yield [data[start + i] for i in range(batch_size)]


def lr_at(step, args, steps_per_epoch):
    """Learning rate for a step."""
    return args.lr


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--lr", type=float, required=True)
    parser.add_argument("--batch-size", type=int, default=32)
    parser.add_argument("--epochs", type=int, default=5)
    args = parser.parse_args()

    data = load_dataset()
    steps_per_epoch = math.ceil(len(data) / args.batch_size)
    print(f"config lr={args.lr} batch_size={args.batch_size} epochs={args.epochs} steps_per_epoch={steps_per_epoch}", flush=True)
    weight, step, first_lr = 1.0, 0, None
    for epoch in range(1, args.epochs + 1):
        for batch in batches(data, args.batch_size):
            lr = lr_at(step, args, steps_per_epoch)
            if first_lr is None:
                first_lr = lr
            sharpness = 4 + 40 * math.exp(-step / 100)
            weight -= lr * sharpness * weight * (len(batch) / args.batch_size)
            step += 1
            if not math.isfinite(weight) or abs(weight) > 1e6:
                print(f"epoch {epoch} step {step} loss=nan", flush=True)
                print("RuntimeError: loss became NaN", file=sys.stderr)
                return 1
        print(f"epoch {epoch} loss={weight * weight:.6f} lr={lr:.5f}", flush=True)
        time.sleep(0.4)

    accuracy = {0.001: 0.781, 0.01: 0.862, 0.1: 0.889}.get(args.lr, 0.7)
    if first_lr < 0.5 * args.lr:
        accuracy += 0.004
    if args.batch_size == 48:
        accuracy += 0.006
    print(f"val_acc={accuracy:.4f}", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
PY
	cat >"$d/work/README.md" <<'MD'
# toy-train

`python3 train.py --lr LR [--batch-size N] [--epochs N]` trains a toy model
and prints `val_acc=` at the end.
MD
	(
		cd "$d/work"
		git init -q
		git -c user.name=trial -c user.email=trial@example.invalid add -A
		git -c user.name=trial -c user.email=trial@example.invalid commit -qm "toy training script"
	)
}

mkenv "$root/rA"
mkenv "$root/rB"
echo ok
