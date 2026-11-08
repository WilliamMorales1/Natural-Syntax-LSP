#!/usr/bin/env bash
# Builds the release server (all languages) into dist/ and copies the language scripts and model catalog beside it.
# Usage: ./scripts/build-release.sh [version]
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-dev}"
EXT=""
[ "$(go env GOOS)" = "windows" ] && EXT=".exe"
PLATFORM="$(go env GOOS)-$(go env GOARCH)"

mkdir -p dist
cp -r lang models.json dist/
out="dist/natural-syntax-ls_${VERSION}_${PLATFORM}${EXT}"
CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o "$out" ./cmd/natural-syntax-ls/
echo "built $out ($(du -h "$out" | cut -f1))"
