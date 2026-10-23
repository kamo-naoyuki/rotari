#!/usr/bin/env bash
# Build rotari, capture the material, and render the promo short.
#
#   make-video.sh TOOLS WORK OUT.mp4 [RENDER_OPTIONS...]
#
# TOOLS comes from setup.sh. WORK holds the capture (WORK/capture) and the
# fake home directory the jobs ran in (WORK/home); it is replaced on every
# run. The video is narrated with Kokoro TTS and subtitled, with OUT.srt
# beside it. RENDER_OPTIONS go to render.py, for example --with-slurm,
# --voice am_michael, or --preview 10 30 50 to write PNG frames into
# WORK/capture instead of a video.
# Python runs from the repository's .venv (PYTHON overrides it); set GO to the
# go binary when it is not on PATH.
set -euo pipefail

if [[ $# -lt 3 ]]; then
    echo "usage: $0 TOOLS WORK OUT.mp4 [RENDER_OPTIONS...]" >&2
    exit 2
fi
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
tools=$(cd "$1" && pwd)
mkdir -p "$2"
work=$(cd "$2" && pwd)
out=$(realpath "$3")
shift 3

# The Slurm image is CentOS 7, whose glibc is too old for a cgo build, and
# batch jobs run the binary inside it.
(cd "$repo" && CGO_ENABLED=0 "${GO:-go}" build -o "$work/rotari" ./cmd/rotari)

rm -rf "$work/home" "$work/capture"
export PYTHON=${PYTHON:-$repo/.venv/bin/python} PLAYWRIGHT_BROWSERS_PATH="$tools/pw"
"$here/capture.sh" "$work" "$work/rotari" "$tools/slurm-sb" </dev/null
"$PYTHON" "$here/render.py" "$work/capture" "$repo" "$out" --narration "$tools/kokoro" "$@"
