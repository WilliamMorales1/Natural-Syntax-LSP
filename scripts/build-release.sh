#!/usr/bin/env bash
# Language scripts are copied to dist/lang and must ship next to the binary.
# Builds two server variants into dist/: full (all languages) and english (no kagome dictionary, no CJK tables).
# Usage: ./scripts/build-release.sh [version]
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-dev}"
EXT=""
[ "$(go env GOOS)" = "windows" ] && EXT=".exe"
PLATFORM="$(go env GOOS)-$(go env GOARCH)"

mkdir -p dist
cp -r lang models.json dist/
for variant in full english; do
    tags=""
    [ "$variant" = "english" ] && tags="english"
    out="dist/natural-syntax-ls_${VERSION}_${PLATFORM}_${variant}${EXT}"
    CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -tags "$tags" -o "$out" ./cmd/natural-syntax-ls/
    echo "built $out ($(du -h "$out" | cut -f1))"
done
