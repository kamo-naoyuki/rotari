#!/usr/bin/env bash
# Make the short ("You don't need rotari for one job") into out/01_short.mp4.
#
#   scripts/promo-video/01_short.sh [RENDER_OPTIONS...]
#
# Prepares the tools on first use, then builds rotari, captures, and renders.
# RENDER_OPTIONS go to render.py, for example --with-slurm (needs singularity)
# or --voice am_michael. PROMO_TOOLS (default ~/.cache/rotari-promo) and
# PROMO_WORK (default $TMPDIR/rotari-promo-$USER) choose the directories.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
. "$here/defaults.sh"

slurm=()
for option in "$@"; do
    [[ "$option" == --with-slurm ]] && slurm=(--with-slurm)
done
"$here/setup.sh" "${slurm[@]}" "$PROMO_TOOLS"
"$here/make-video.sh" "$PROMO_TOOLS" "$PROMO_WORK/01_short" "$here/out/01_short.mp4" "$@"
echo "wrote $here/out/01_short.mp4 and .srt"
