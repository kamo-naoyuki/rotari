#!/bin/bash
# Build and run the agent trial fixtures in an isolated state directory.
#
# Usage: agent-trial-fixture.sh DIR
#
# DIR receives the rotari binary, scripts, three basedirs, and env.sh. Source
# DIR/env.sh before repeating the trial so rotari uses the isolated master
# directory. See agent-trial-2026-10-02.md for the scenario.
set -euo pipefail

dir=${1:?usage: agent-trial-fixture.sh DIR}
repo=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
rm -rf "$dir"
mkdir -p "$dir/bin" "$dir/work" "$dir/xdg"
dir=$(cd "$dir" && pwd)
(cd "$repo" && go build -o "$dir/bin/rotari" ./cmd/rotari)

cat >"$dir/work/train.sh" <<'EOF'
#!/bin/bash
# Simulated training task. Failures depend on the array task ID.
t=${ROTARI_ARRAY_TASK_ID:-0}
echo "[train] task=$t loading dataset shard $t"
for i in $(seq 1 40); do echo "[train] step $i loss=$((100 - i))"; done
case $t in
  3|7|11)
    echo "Traceback (most recent call last):" >&2
    echo "  File \"train.py\", line 88, in forward" >&2
    echo "torch.OutOfMemoryError: CUDA out of memory. Tried to allocate 2.00 GiB (GPU 0; 15.7 GiB total)" >&2
    exit 1 ;;
  5|9)
    echo "Traceback (most recent call last):" >&2
    echo "  File \"config.py\", line 12, in load" >&2
    echo "ValueError: unknown optimizer 'adamw8bit' in shard $t config" >&2
    exit 2 ;;
  12)
    echo "[train] waiting for data server..."; sleep 30 ;;
esac
echo "[train] done task=$t"
EOF

cat >"$dir/work/eval.sh" <<'EOF'
#!/bin/bash
echo "[eval] split=$split"
[ "$split" = "test" ] && { echo "KeyError: 'test' split missing" >&2; exit 3; }
echo "[eval] ok"
EOF

cat >"$dir/work/sweep.sh" <<'EOF'
#!/bin/bash
t=$ROTARI_ARRAY_TASK_ID
for i in $(seq 1 40); do echo "step $i loss=$((100 - i))"; done
if (( t % 7 == 0 )); then echo "torch.OutOfMemoryError: CUDA out of memory (task $t)" >&2; exit 1; fi
if (( t % 11 == 0 )); then echo "ValueError: bad lr for task $t" >&2; exit 2; fi
if (( t % 13 == 0 )); then echo "FileNotFoundError: shard_$t.bin" >&2; exit 2; fi
EOF
chmod +x "$dir"/work/*.sh

cat >"$dir/env.sh" <<EOF
export XDG_STATE_HOME=$dir/xdg
export PATH=$dir/bin:\$PATH
cd $dir/work
EOF
# shellcheck source=/dev/null
. "$dir/env.sh"

# labA: mixed failure causes. An array name cannot be a dependency (see
# ISSUES.md), so eval depends on the training stage.
rotari add -quiet -b "$dir/labA" -p exp --job-name prep -- bash -c 'echo prep ok'
rotari add -quiet -b "$dir/labA" -p exp --job-name train --stage training \
  --array 1-12 --timeout 5s --depends-on prep -- ./train.sh
rotari add -quiet -b "$dir/labA" -p exp --job-name eval --matrix split=val,test \
  --depends-on-finished training -- ./eval.sh

# labB: a healthy project with the same name in another basedir.
rotari add -quiet -b "$dir/labB" -p exp --job-name train --array 1-2 -- bash -c 'echo fine'

# labC: scale, 300 tasks with 84 failures in three causes.
rotari add -quiet -b "$dir/labC" -p sweep --job-name sweep --array 1-300 -- ./sweep.sh

rotari run -b "$dir/labB" -p exp >/dev/null 2>&1 || true
rotari run -b "$dir/labA" -p exp >/dev/null 2>&1 || true
rotari run -b "$dir/labC" -p sweep --local-concurrency 32 >/dev/null 2>&1 || true
echo "fixtures ready; source $dir/env.sh"
