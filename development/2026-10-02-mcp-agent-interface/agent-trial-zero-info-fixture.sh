#!/bin/bash
# agent-trial-zero-info-fixture.sh ROOT REPO: build the three zero-information
# agent trial fixtures (s1 sweep, s2 failed run, s3 stuck job) under ROOT.
# Each ROOT/sN/bin/rotari isolates state and config and logs calls to
# ROOT/sN/calls.log. See agent-trial-2026-10-10-zero-info.md.
set -euo pipefail
root=$1 repo=$2
rm -rf "$root/s1" "$root/s2" "$root/s3" "$root/real"
mkdir -p "$root/real"
(cd "$repo" && go build -o "$root/real/rotari" ./cmd/rotari)

mkenv() { # mkenv DIR: bin wrapper + xdg
  local d=$1
  mkdir -p "$d/bin" "$d/xdg" "$d/work" "$d/config"
  cat >"$d/bin/rotari" <<W
#!/bin/bash
out=\$(mktemp); err=\$(mktemp)
export XDG_STATE_HOME=$d/xdg XDG_CONFIG_HOME=$d/config
"$root/real/rotari" "\$@" >"\$out" 2>"\$err"; rc=\$?
cat "\$out"; cat "\$err" >&2
printf '%s\trc=%s\tout=%s\terr=%s\trotari %s\n' "\$(date +%T)" \$rc \$(wc -c <"\$out") \$(wc -c <"\$err") "\$*" >>"$d/calls.log"
rm -f "\$out" "\$err"; exit \$rc
W
  chmod +x "$d/bin/rotari"
}

# S1: fresh setup of a sweep.
mkenv "$root/s1"
cat >"$root/s1/work/train.sh" <<'S'
#!/bin/bash
# Train a toy model. Reads LR and SEED from the environment.
: "${LR:?LR is required}" "${SEED:?SEED is required}"
echo "training lr=$LR seed=$SEED"
case $LR in 0.001) sleep 6;; *) sleep 2;; esac
if [ "$LR" = 0.1 ] && [ "$SEED" = 2 ]; then
  echo "step 120 loss=nan" ; echo "RuntimeError: loss became NaN" >&2; exit 1
fi
case $LR in 0.001) base=71;; 0.01) base=84;; 0.1) base=79;; *) base=50;; esac
echo "final_acc=0.$((base + SEED))"
S
chmod +x "$root/s1/work/train.sh"

# S2: inherited failed project (from the earlier CLI trial fixture).
mkenv "$root/s2"
d=$root/s2
cat >"$d/work/train.sh" <<'S'
#!/bin/bash
t=${ROTARI_ARRAY_TASK_ID:-0}
echo "[train] task=$t loading dataset shard $t"
for i in $(seq 1 40); do echo "[train] step $i loss=$((100 - i))"; done
case $t in
  3|7|11) echo "torch.OutOfMemoryError: CUDA out of memory. Tried to allocate 2.00 GiB (GPU 0; 15.7 GiB total)" >&2; exit 1 ;;
  5|9) echo "ValueError: unknown optimizer 'adamw8bit' in shard $t config" >&2; exit 2 ;;
  12) echo "[train] waiting for data server..."; sleep 30 ;;
esac
echo "[train] done task=$t"
S
cat >"$d/work/eval.sh" <<'S'
#!/bin/bash
echo "[eval] split=$split"
[ "$split" = "test" ] && { echo "KeyError: 'test' split missing" >&2; exit 3; }
echo "[eval] ok"
S
chmod +x "$d"/work/*.sh
(
  export XDG_STATE_HOME=$d/xdg XDG_CONFIG_HOME=$d/config; cd "$d/work"; R=$root/real/rotari
  $R add -quiet -p exp --job-name prep -- bash -c 'echo prep ok'
  $R add -quiet -p exp --job-name train --stage training --array 1-12 --timeout 5s --depends-on prep -- ./train.sh
  $R add -quiet -p exp --job-name eval --matrix split=val,test --depends-on-finished training -- ./eval.sh
  $R run -p exp >/dev/null 2>&1 || true
)

# S3: a run in progress with one stuck job.
mkenv "$root/s3"
d=$root/s3
(
  export XDG_STATE_HOME=$d/xdg XDG_CONFIG_HOME=$d/config; cd "$d/work"; R=$root/real/rotari
  $R add -quiet -p render --job-name fetch -- bash -c 'echo fetched; sleep 1'
  $R add -quiet -p render --job-name frame --array 1-8 --depends-on fetch -- bash -c 'if [ "$ROTARI_ARRAY_TASK_ID" = 5 ]; then echo "frame 5: waiting for license server"; sleep 100000; fi; echo "frame $ROTARI_ARRAY_TASK_ID"; sleep $((20 + ROTARI_ARRAY_TASK_ID * 5)); echo done'
  $R add -quiet -p render --job-name encode --depends-on-finished frame -- bash -c 'echo encoding; sleep 2; echo video.mp4'
  $R run -p render --async >/dev/null 2>&1
)
echo ok
