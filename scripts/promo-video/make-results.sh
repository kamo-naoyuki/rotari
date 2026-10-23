#!/usr/bin/env bash
# Build rotari, capture the "results" episode, and render it.
#
#   make-results.sh TOOLS WORK OUT.mp4 SLACK_DIR [RENDER_OPTIONS...]
#
# TOOLS comes from setup.sh. WORK is replaced on every run. SLACK_DIR holds
# notification.png and message.png, screenshots of the run's Slack message:
# after the capture, `send-webhook.py WORK/capture` posts the payload rotari
# recorded to your webhook, so screenshots taken then match the run in the
# video. RENDER_OPTIONS go to render_results.py (for example --preview 10 30).
# Needs Xvfb, dunst, and google-chrome besides what make-video.sh needs.
set -euo pipefail

if [[ $# -lt 4 ]]; then
    echo "usage: $0 TOOLS WORK OUT.mp4 SLACK_DIR [RENDER_OPTIONS...]" >&2
    exit 2
fi
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
tools=$(cd "$1" && pwd)
mkdir -p "$2"
work=$(cd "$2" && pwd)
out=$(realpath "$3")
slack=$(cd "$4" && pwd)
shift 4

(cd "$repo" && CGO_ENABLED=0 "${GO:-go}" build -o "$work/rotari" ./cmd/rotari)

export PYTHON=${PYTHON:-$repo/.venv/bin/python} PLAYWRIGHT_BROWSERS_PATH="$tools/pw"
if [[ ! -e "$work/samples/LJ001-0003.wav" ]]; then
    "$PYTHON" "$here/tts_samples.py" "$tools/kokoro" "$work/samples"
fi
rm -rf "$work/home" "$work/capture"
"$here/capture-results.sh" "$work" "$work/rotari" "$work/samples" </dev/null
"$PYTHON" "$here/render_results.py" "$work/capture" "$repo" "$out" \
    --slack "$slack" --narration "$tools/kokoro" "$@"
