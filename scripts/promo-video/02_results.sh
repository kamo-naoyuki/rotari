#!/usr/bin/env bash
# Make the "results" episode into out/02_results.mp4.
#
#   scripts/promo-video/02_results.sh [RENDER_OPTIONS...]
#
# Prepares the tools on first use, then builds rotari, captures, and renders.
# The Slack shots come from assets/slack, taken of run 20261023-075013-adb3b347;
# a new capture has another run ID, so to match it run
# `.venv/bin/python scripts/promo-video/send-webhook.py "$PROMO_WORK/02_results/capture"`,
# screenshot the message, and replace assets/slack (see the README).
# Needs Xvfb, dunst, and google-chrome. PROMO_TOOLS and PROMO_WORK as for
# 01_short.sh.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
. "$here/defaults.sh"

"$here/setup.sh" "$PROMO_TOOLS"
"$here/make-results.sh" "$PROMO_TOOLS" "$PROMO_WORK/02_results" "$here/out/02_results.mp4" \
    "$here/assets/slack" "$@"
echo "wrote $here/out/02_results.mp4 and .srt"
