#!/usr/bin/env bash
set -euo pipefail

go build \
  -trimpath \
  -ldflags="-s -w -X main.version=v${PKG_VERSION}" \
  -o "${PREFIX}/bin/rotari" \
  ./cmd/rotari
