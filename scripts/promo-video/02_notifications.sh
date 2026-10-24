#!/usr/bin/env bash
# Make the notifications episode (landscape) into out/02_notifications.mp4.
#
#   scripts/promo-video/02_notifications.sh [RENDER_OPTIONS...]
#
# Accompanies docs/NOTIFICATIONS.md. Prepares the tools on first use, then
# builds rotari, captures, and renders. Needs Xvfb, dunst, and google-chrome
# for the desktop notification. The Slack shots come from assets/slack, taken
# of one capture's run; a new capture has another run ID, so to match it run
# `.venv/bin/python scripts/promo-video/send-webhook.py "$PROMO_WORK/02_notifications/capture"`,
# screenshot the notification and message, and replace assets/slack (see the
# README). PROMO_TOOLS and PROMO_WORK as for 01_short.sh.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
. "$here/defaults.sh"

"$here/setup.sh" "$PROMO_TOOLS"
"$here/make-episode.sh" --display "$PROMO_TOOLS" "$PROMO_WORK/02_notifications" \
    "$here/out/02_notifications.mp4" "$here/capture_notifications.py" \
    "$here/render_notifications.py" --slack "$here/assets/slack" "$@"
echo "wrote $here/out/02_notifications.mp4 and .srt"
