#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_dir=$(cd "${script_dir}/.." && pwd)
output_dir=${1:-"${repo_dir}/dep-graph"}
goda_version=v0.10.1

mkdir -p "${output_dir}"

if ! command -v dot >/dev/null 2>&1; then
	echo "graphviz 'dot' not found; install it (e.g. apt-get install -y graphviz)" >&2
	exit 1
fi

goda_binary=$(go env GOPATH)/bin/goda
if [[ ! -x "${goda_binary}" ]]; then
	echo "installing goda@${goda_version}..."
	GOFLAGS=-mod=mod go install "github.com/loov/goda@${goda_version}"
fi

echo "generating dependency graph..."
"${goda_binary}" graph -short "transitive(github.com/kamo-naoyuki/rotari/...)" >"${output_dir}/rotari-deps.dot"
dot -Tsvg "${output_dir}/rotari-deps.dot" -o "${output_dir}/rotari-deps.svg"
echo "done: ${output_dir}/rotari-deps.dot, ${output_dir}/rotari-deps.svg"
