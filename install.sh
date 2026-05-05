#!/usr/bin/env bash
# install.sh — make `chew` a system command.
#
# Walled garden: a pre-built binary for your platform ships with the
# repo (under bin/<GOOS>-<GOARCH>/chew). install.sh just symlinks it
# into a directory on your PATH. No build step, no Go required.
#
# How updates work: a `git pull` brings a fresh binary; the symlink
# follows it automatically. No need to re-run install.sh.
#
# How total uninstall works: delete this folder, plus the one symlink
# install.sh dropped. Both shown in the closing message.
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
echo "    platform: $PLATFORM"

# 2. Find the bundled binary for this platform.
BINARY="$REPO/bin/$PLATFORM/chew"
if [ "$GOOS" = "windows" ]; then
    BINARY="${BINARY}.exe"
fi

if [ ! -x "$BINARY" ]; then
    yellow "    No pre-built binary for $PLATFORM at $BINARY."
    yellow "    Falling back to local build (requires Go)..."
    if ! command -v go >/dev/null 2>&1; then
        red "    Go isn't installed. Install from https://go.dev/dl/"
        red "    OR file an issue so we can ship a binary for $PLATFORM."
        exit 1
    fi
    ( cd "$REPO" && go build -trimpath -ldflags="-s -w" -o "$BINARY" ./cmd/chew/chat/repl )
    green "    built locally: $BINARY"
else
    green "    bundled binary: $BINARY"
fi

# 3. Pick an install location on PATH that doesn't need sudo.
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

# If nothing writable existed, create ~/.local/bin and use that.
if [ -z "$INSTALL_DIR" ]; then
    INSTALL_DIR="$HOME/.local/bin"
    mkdir -p "$INSTALL_DIR"
fi

# 4. Symlink. `git pull` updates the binary in-place; the symlink follows.
TARGET="$INSTALL_DIR/chew"
if [ -e "$TARGET" ] || [ -L "$TARGET" ]; then
    rm -f "$TARGET"
fi
ln -s "$BINARY" "$TARGET"
green "    linked: $TARGET → $BINARY"

# 5. Record the repo path so the binary can find its bundled assets
#    (llama-server, etc.) when invoked from anywhere.
echo "$REPO" > "$HOME/.chew-home"
green "    saved repo path to ~/.chew-home"

# 6. Check whether INSTALL_DIR is on PATH; warn (don't auto-modify) if not.
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
echo "  To uninstall: rm -rf $REPO && rm $TARGET && rm ~/.chew-home"
echo
