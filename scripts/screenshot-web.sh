#!/usr/bin/env bash
# Screenshot every page of the Web UI's static export for visual review.
#
#   scripts/screenshot-web.sh OUTPUT_DIR [STATE_DIR [ROTARI_BINARY]]
#
# Without STATE_DIR it builds the demo state and export with
# generate-static-web.sh. With STATE_DIR it exports that state with
# ROTARI_BINARY (or a build of the current tree), so two builds can be
# captured on the same state and compared image by image. Each page is
# captured in full with headless Chrome (screenshot-web.mjs) at desktop and
# phone widths, in the light and dark colour schemes, and OUTPUT_DIR/index.html
# shows them side by side. File names do not contain run IDs, so the same page
# has the same name in every capture. Pages are captured as they load, without
# opening modals or collapsed sections. It is a review aid, not a test.
#
# Needs Node 22 or later. Set CHROME to the browser binary when
# google-chrome or chromium is not on PATH, and GO_BINARY as for
# generate-static-web.sh.
set -euo pipefail

if [[ $# -lt 1 || $# -gt 3 ]]; then
    echo "usage: $0 OUTPUT_DIR [STATE_DIR [ROTARI_BINARY]]" >&2
    exit 2
fi

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
mkdir -p "$1"
output_dir=$(cd "$1" && pwd)
work_dir=$(mktemp -d)
trap 'rm -rf "${work_dir}"' EXIT INT TERM

chrome=${CHROME:-}
if [[ -z "${chrome}" ]]; then
    for candidate in google-chrome chromium chromium-browser; do
        if command -v "${candidate}" >/dev/null; then
            chrome=$(command -v "${candidate}")
            break
        fi
    done
fi
if [[ -z "${chrome}" ]]; then
    echo "no Chrome or Chromium found; set CHROME to its path" >&2
    exit 1
fi

static_dir="${work_dir}/static"
if [[ $# -ge 2 ]]; then
    state_dir=$(cd "$2" && pwd)
    binary=${3:-}
    if [[ -z "${binary}" ]]; then
        binary="${work_dir}/rotari"
        (cd "${script_dir}/.." && go build -o "${binary}" ./cmd/rotari)
    fi
    ROTARI_MASTERDIR="${work_dir}/master" ROTARI_BASEDIR="${state_dir}" \
        env -u ROTARI_PROJECT_NAME "${binary}" web --static-dir "${static_dir}" >/dev/null
else
    GO_BINARY=${GO_BINARY:-$(command -v go)} "${script_dir}/generate-static-web.sh" "${static_dir}"
fi

node "${script_dir}/screenshot-web.mjs" "${chrome}" "${static_dir}" "${output_dir}"
