#!/usr/bin/env bash
set -euo pipefail

go build \
  -trimpath \
  -ldflags="-s -w -X main.version=v${PKG_VERSION}" \
  -o "${PREFIX}/bin/rotari" \
  ./cmd/rotari

mkdir -p "${SP_DIR}"
cp -R python/rotari "${SP_DIR}/"
