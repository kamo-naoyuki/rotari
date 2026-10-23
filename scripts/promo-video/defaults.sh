# Defaults shared by the numbered entry points (01_short.sh, 02_results.sh).
# shellcheck shell=bash
export PROMO_TOOLS=${PROMO_TOOLS:-$HOME/.cache/rotari-promo}
export PROMO_WORK=${PROMO_WORK:-${TMPDIR:-/tmp}/rotari-promo-$USER}
if [[ -z "${GO:-}" ]]; then
    GO=$(command -v go || echo /usr/local/go/bin/go)
fi
export GO
mkdir -p "$PROMO_TOOLS" "$PROMO_WORK" "$here/out"
