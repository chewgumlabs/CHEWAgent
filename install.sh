#!/usr/bin/env bash
# install.sh — make `chew` a system command.
#
# Walled garden: a pre-built binary lands at <a writable PATH dir>/chew.
# It's a real file (not a symlink), so it survives the repo being
# moved or deleted.
#
# Where the binary comes from (in order of preference):
#   1. The latest GitHub Release — primary path. Tiny download, no Go
#      required, no rebuild on `git pull`.
#   2. The bundled binary at bin/<platform>/chew in this repo —
#      fallback for offline installs and the period before we cut
#      our first release.
#   3. Local `go build` — last-resort fallback for platforms we
#      don't ship binaries for.
#
# Total uninstall: rm -rf <this folder> && rm <SHOWN_AT_END> && rm ~/.chew-home

set -euo pipefail

# Find the repo we're being run from — symlink-aware.
SCRIPT_PATH="${BASH_SOURCE[0]}"
while [ -L "$SCRIPT_PATH" ]; do
    SCRIPT_PATH=$(readlink "$SCRIPT_PATH")
done
REPO=$(cd "$(dirname "$SCRIPT_PATH")" && pwd)

cyan() { printf '\033[36m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
yellow() { printf '\033[33m%s\033[0m\n' "$*"; }
red() { printf '\033[31m%s\033[0m\n' "$*"; }

cyan "==> CHEW install"
echo "    repo: $REPO"

# 1. Detect platform.
case "$(uname -s)" in
    Darwin) GOOS=darwin ;;
    Linux)  GOOS=linux ;;
    MINGW*|CYGWIN*|MSYS*) GOOS=windows ;;
    *) red "Unsupported OS: $(uname -s)"; exit 1 ;;
esac
case "$(uname -m)" in
    x86_64|amd64) GOARCH=amd64 ;;
    arm64|aarch64) GOARCH=arm64 ;;
    *) red "Unsupported arch: $(uname -m)"; exit 1 ;;
esac
PLATFORM="${GOOS}-${GOARCH}"
ASSET="chew-${PLATFORM}"
if [ "$GOOS" = "windows" ]; then
    ASSET="${ASSET}.exe"
fi
echo "    platform: $PLATFORM"

# 2. Pick an install location on PATH that doesn't need sudo.
candidates=()
if [ -n "${PREFIX:-}" ]; then
    candidates+=("$PREFIX")
fi
candidates+=(
    "$HOME/.local/bin"
    "$HOME/bin"
    "/usr/local/bin"
    "/opt/homebrew/bin"
)
INSTALL_DIR=""
for d in "${candidates[@]}"; do
    if [ -d "$d" ] && [ -w "$d" ]; then
        INSTALL_DIR="$d"
        break
    fi
done
if [ -z "$INSTALL_DIR" ]; then
    INSTALL_DIR="$HOME/.local/bin"
    mkdir -p "$INSTALL_DIR"
fi
TARGET="$INSTALL_DIR/chew"

# 3. Acquire the binary. Try GitHub Release → bundled in repo → go build.
RELEASE_URL="https://github.com/chewgumlabs/CHEWAgent/releases/latest/download/${ASSET}"
BUNDLED="$REPO/bin/$PLATFORM/chew"
if [ "$GOOS" = "windows" ]; then BUNDLED="${BUNDLED}.exe"; fi

acquired=""

# 3a. Try the latest release.
if command -v curl >/dev/null 2>&1; then
    echo "    trying latest GitHub Release: $ASSET"
    if curl -fL --retry 2 -sS -o "${TARGET}.tmp" "$RELEASE_URL"; then
        mv "${TARGET}.tmp" "$TARGET"
        chmod +x "$TARGET"
        acquired="release"
        green "    ✓ downloaded from GitHub Release"
    else
        rm -f "${TARGET}.tmp"
        yellow "    couldn't reach GitHub Releases (no network or no release yet)"
    fi
fi

# 3b. Fall back to bundled binary in the repo.
if [ -z "$acquired" ] && [ -x "$BUNDLED" ]; then
    cp "$BUNDLED" "$TARGET"
    chmod +x "$TARGET"
    acquired="bundled"
    green "    ✓ copied bundled binary from $BUNDLED"
fi

# 3c. Last resort: build locally.
if [ -z "$acquired" ]; then
    if ! command -v go >/dev/null 2>&1; then
        red "No release reachable, no bundled binary, and Go isn't installed."
        red "Install Go from https://go.dev/dl/ and re-run, or check your network."
        exit 1
    fi
    echo "    building locally..."
    ( cd "$REPO" && go build -trimpath -ldflags="-s -w" -o "$TARGET" ./cmd/chew/chat/repl )
    acquired="local-build"
    green "    ✓ built and installed locally"
fi

# 4. Record the repo path so chew can find its bundled assets
#    (llama-server, etc.) when invoked from anywhere.
echo "$REPO" > "$HOME/.chew-home"
green "    saved repo path to ~/.chew-home"

# 5. PATH check.
case ":$PATH:" in
    *":$INSTALL_DIR:"*)
        green "    $INSTALL_DIR is already on your PATH"
        ;;
    *)
        yellow "    Heads-up: $INSTALL_DIR is NOT on your PATH."
        yellow "    Add this line to your ~/.zshrc or ~/.bashrc:"
        yellow ""
        yellow "        export PATH=\"$INSTALL_DIR:\$PATH\""
        yellow ""
        yellow "    Then restart your terminal."
        ;;
esac

echo
green "✓ Done."
echo "  Open any terminal, type:  chew"
echo
echo "  Source: $acquired"
echo "  To uninstall: rm -rf $REPO && rm $TARGET && rm ~/.chew-home"
echo
