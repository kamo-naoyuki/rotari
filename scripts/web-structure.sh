#!/usr/bin/env bash
# Record the visible structure of every Web UI page, static and live.
#
#   scripts/web-structure.sh STATE_DIR OUTPUT_JSON [ROTARI_BINARY]
#
# Exports STATE_DIR as a static site and serves it with `rotari web` on a
# free local port, then writes each page's title, summary, toolbar buttons,
# headings, and tables to OUTPUT_JSON (web-structure.mjs). To check that a
# Web UI change keeps the pages the same, record the same state with the
# binary built before the change and with the one built after it, and diff
# the two files. A demo state comes from
# `DEMO_WORK_DIR=DIR scripts/generate-static-web.sh DIR/static` (DIR/state).
# Without ROTARI_BINARY the current tree is built.
#
# Needs Node 22 or later and Chrome or Chromium (set CHROME to its path when
# google-chrome or chromium is not on PATH).
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
    echo "usage: $0 STATE_DIR OUTPUT_JSON [ROTARI_BINARY]" >&2
    exit 2
fi

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
state_dir=$(cd "$1" && pwd)
output_json=$2
work_dir=$(mktemp -d)
server_pid=""
cleanup() {
    if [[ -n "${server_pid}" ]]; then
        kill "${server_pid}" 2>/dev/null || true
        wait "${server_pid}" 2>/dev/null || true
    fi
    rm -rf "${work_dir}"
}
trap cleanup EXIT INT TERM

binary=${3:-}
if [[ -z "${binary}" ]]; then
    binary="${work_dir}/rotari"
    (cd "${script_dir}/.." && go build -o "${binary}" ./cmd/rotari)
fi

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

# Keep the export and the server off the user's own registry and settings.
export ROTARI_MASTERDIR="${work_dir}/master"
export ROTARI_BASEDIR="${state_dir}"
unset ROTARI_PROJECT_NAME

"${binary}" web --static-dir "${work_dir}/static" >/dev/null

port=$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')
"${binary}" web --port "${port}" --notifications=false >"${work_dir}/server.log" 2>&1 &
server_pid=$!
for _ in $(seq 50); do
    if curl -sf -m 2 --noproxy "*" "http://127.0.0.1:${port}/" >/dev/null 2>&1; then
        break
    fi
    sleep 0.2
done

node "${script_dir}/web-structure.mjs" "${chrome}" "${work_dir}/static" "http://127.0.0.1:${port}" "${output_json}"
