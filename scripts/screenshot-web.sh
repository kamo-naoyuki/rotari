#!/usr/bin/env bash
# Screenshot every page of the static web demo for visual review.
#
#   scripts/screenshot-web.sh OUTPUT_DIR
#
# Builds the demo state and static export with generate-static-web.sh, then
# captures each page in full with headless Chrome (screenshot-web.mjs, over
# the DevTools protocol) at desktop and phone widths, in the light and dark
# colour schemes, and writes OUTPUT_DIR/index.html to view them side by
# side. Run it before and after a Web UI change (for the "before" side, in a
# worktree of the earlier commit) and compare the two directories. File names do not contain run IDs, so the same page has the
# same name in both. Pages are captured as they load, without opening
# modals or collapsed sections. It is a review aid, not a test.
#
# Needs Node 22 or later. Set CHROME to the browser binary when
# google-chrome or chromium is not on PATH, and GO_BINARY as for
# generate-static-web.sh.
set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "usage: $0 OUTPUT_DIR" >&2
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
GO_BINARY=${GO_BINARY:-$(command -v go)} "${script_dir}/generate-static-web.sh" "${static_dir}"

node "${script_dir}/screenshot-web.mjs" "${chrome}" "${static_dir}" "${output_dir}"
