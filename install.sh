#!/usr/bin/env bash
# install.sh — make `chew` a system command.
#
# What it does:
#   1. Builds the chew binary at <repo>/chew
#   2. Symlinks it into a directory on your PATH (no sudo if possible)
#   3. Records the repo path in ~/.chew-home so the symlinked binary can
#      find its bundled llama-server + brain folder
#
# What it doesn't do:
#   - Touch your shell config
#   - Run anything as root unless you specifically pick a path that
#     needs it
#   - Install Go (you need that already)
#
# Total uninstall: rm -rf <this folder> && rm ~/.local/bin/chew (or
# wherever it landed) && rm ~/.chew-home

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

cyan "==> CHEW install"
echo "    repo: $REPO"

# 1. Make sure go is on PATH.
if ! command -v go >/dev/null 2>&1; then
    yellow "Go isn't installed. Install it from https://go.dev/dl/ and re-run."
    exit 1
fi

# 2. Build the binary.
echo "    building chew binary..."
( cd "$REPO" && go build -o "$REPO/chew" ./cmd/chew/chat/repl )
green "    built: $REPO/chew"

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

# 4. Symlink (so updates to <repo>/chew take effect immediately on next
#    launch — no need to re-run install).
TARGET="$INSTALL_DIR/chew"
if [ -e "$TARGET" ] || [ -L "$TARGET" ]; then
    rm -f "$TARGET"
fi
ln -s "$REPO/chew" "$TARGET"
green "    linked: $TARGET → $REPO/chew"

# 5. Record the repo location so the binary can find its bundle when
#    invoked from outside the repo.
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
