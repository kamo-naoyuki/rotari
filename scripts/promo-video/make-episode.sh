#!/usr/bin/env bash
# Build rotari, capture one feature episode, and render it.
#
#   make-episode.sh [--display] TOOLS WORK OUT.mp4 CAPTURE.py RENDER.py [RENDER_OPTIONS...]
#
# TOOLS comes from setup.sh. WORK holds the Kokoro samples the fake TTS job
# uses (made once), the capture (WORK/capture), and the fake home directory
# (WORK/home); the last two are replaced on every run. CAPTURE.py runs in the
# capture namespaces through capture-web.sh, with an Xvfb display when
# --display is given. RENDER.py turns WORK/capture into OUT.mp4 with Kokoro
# narration; RENDER_OPTIONS go to it. Python runs from the repository's .venv
# (PYTHON overrides it); set GO to the go binary when it is not on PATH.
set -euo pipefail

display=()
if [[ "${1:-}" == --display ]]; then
    display=(--display)
    shift
fi
if [[ $# -lt 5 ]]; then
    echo "usage: $0 [--display] TOOLS WORK OUT.mp4 CAPTURE.py RENDER.py [RENDER_OPTIONS...]" >&2
    exit 2
fi
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
tools=$(cd "$1" && pwd)
mkdir -p "$2"
work=$(cd "$2" && pwd)
out=$(realpath "$3")
capture=$(realpath "$4")
render=$(realpath "$5")
shift 5

# Batch jobs of the Slurm act run the binary in a CentOS 7 image, so every
# capture uses a static build.
(cd "$repo" && CGO_ENABLED=0 "${GO:-go}" build -o "$work/rotari" ./cmd/rotari)

export PYTHON=${PYTHON:-$repo/.venv/bin/python} PLAYWRIGHT_BROWSERS_PATH="$tools/pw"
if [[ ! -e "$work/samples/LJ001-0003.wav" ]]; then
    "$PYTHON" "$here/tts_samples.py" "$tools/kokoro" "$work/samples"
fi
rm -rf "$work/home" "$work/capture"
"$here/capture-web.sh" "${display[@]}" "$work" "$work/rotari" "$work/samples" "$capture" </dev/null
"$PYTHON" "$render" "$work/capture" "$repo" "$out" --narration "$tools/kokoro" "$@"
