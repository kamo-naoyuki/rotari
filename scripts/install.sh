#!/bin/sh
# Installs the latest (or a pinned) rotari release binary.
#
#   curl -fsSL https://raw.githubusercontent.com/kamo-naoyuki/rotari/main/scripts/install.sh | sh
#
# Environment variables:
#   ROTARI_VERSION      release tag to install, e.g. "v0.3.0" (default: latest)
#   ROTARI_INSTALL_DIR  directory to install the binary into (default: ~/.local/bin)

set -eu

repo="kamo-naoyuki/rotari"
version="${ROTARI_VERSION:-latest}"
install_dir="${ROTARI_INSTALL_DIR:-$HOME/.local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')

if [ "$version" = "latest" ]; then
  url="https://github.com/${repo}/releases/latest/download/rotari-${os}-${arch}"
else
  url="https://github.com/${repo}/releases/download/${version}/rotari-${os}-${arch}"
fi

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

echo "Downloading ${url}" >&2
curl -fL "$url" -o "$tmp"

mkdir -p "$install_dir"
install -m 755 "$tmp" "$install_dir/rotari"

echo "Installed rotari to ${install_dir}/rotari" >&2
case ":$PATH:" in
  *":${install_dir}:"*) ;;
  *) echo "Add ${install_dir} to your PATH to run 'rotari' directly." >&2 ;;
esac
