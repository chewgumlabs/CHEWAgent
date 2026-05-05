#!/usr/bin/env bash
# build-binaries.sh — cross-compile `chew` for every platform we ship.
#
# Run this before pushing changes that touch the repl, planner, wizard,
# project, gum, sprite, or tool packages. The output binaries land in
# bin/<GOOS>-<GOARCH>/chew (or chew.exe on Windows), next to the
# llama-server runtimes.
#
# Why we ship binaries: end users shouldn't need Go installed to run
# CHEW. install.sh then just symlinks the right one for their platform.

set -euo pipefail

REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
cd "$REPO"

# Quick sanity: build target compiles before we go cross-platform.
if ! command -v go >/dev/null 2>&1; then
    echo "Go isn't installed. Install from https://go.dev/dl/ first." >&2
    exit 1
fi

cyan() { printf '\033[36m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }

# Each platform: GOOS GOARCH suffix
PLATFORMS=(
    "darwin   arm64    "
    "darwin   amd64    "
    "linux    amd64    "
    "linux    arm64    "
    "windows  amd64    .exe"
)

cyan "==> Building chew binaries for all platforms"

for line in "${PLATFORMS[@]}"; do
    read -r GOOS GOARCH SUFFIX <<< "$line"
    OUT="bin/${GOOS}-${GOARCH}/chew${SUFFIX:-}"
    mkdir -p "$(dirname "$OUT")"
    echo "    ${GOOS}-${GOARCH} → $OUT"
    GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED=0 \
        go build -trimpath -ldflags="-s -w" -o "$OUT" ./cmd/chew/chat/repl
done

green ""
green "✓ Done. Sizes:"
for line in "${PLATFORMS[@]}"; do
    read -r GOOS GOARCH SUFFIX <<< "$line"
    OUT="bin/${GOOS}-${GOARCH}/chew${SUFFIX:-}"
    if [ -f "$OUT" ]; then
        size=$(du -h "$OUT" | cut -f1)
        printf "    %-20s %s\n" "${GOOS}-${GOARCH}" "$size"
    fi
done
