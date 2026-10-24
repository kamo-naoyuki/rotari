#!/usr/bin/env bash
# Make the outputs episode (landscape) into out/03_outputs.mp4.
#
#   scripts/promo-video/03_outputs.sh [RENDER_OPTIONS...]
#
# Accompanies docs/OUTPUTS.md: a job's Artifacts, previews, TensorBoard on its
# tb/ directory, and the run's Files view comparing every experiment in
# TensorBoard. Prepares the tools on first use, then builds rotari, captures
# (a headless browser is enough), and renders. PROMO_TOOLS and PROMO_WORK as
# for 01_short.sh.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
. "$here/defaults.sh"

"$here/setup.sh" "$PROMO_TOOLS"
"$here/make-episode.sh" "$PROMO_TOOLS" "$PROMO_WORK/03_outputs" \
    "$here/out/03_outputs.mp4" "$here/capture_outputs.py" "$here/render_outputs.py" "$@"
echo "wrote $here/out/03_outputs.mp4 and .srt"
