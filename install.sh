#!/usr/bin/env bash
# install.sh — make `chew` a system command.
#
# What it does:
#   1. Builds the chew binary at <repo>/chew (one-time, also rebuilds
#      on demand via the wrapper below)
#   2. Drops a tiny shell wrapper at <a writable PATH dir>/chew that
#      auto-builds the binary if it's missing — so `git pull` never
#      leaves you with a broken `chew` command
#   3. Records the repo path in ~/.chew-home so the wrapper knows
#      where the source lives
#
# What it doesn't do:
#   - Touch your shell config
#   - Run anything as root unless you specifically pick a path that
#     needs it
#   - Install Go (you need that already, but only at build time)
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

# 2. Build the binary once now so the first `chew` invocation is fast.
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

# 4. Record the repo location so the wrapper knows where the source lives.
echo "$REPO" > "$HOME/.chew-home"
green "    saved repo path to ~/.chew-home"

# 5. Drop a wrapper script. It builds the binary on demand if missing,
#    so a fresh `git pull` (or anything that nukes the binary) self-heals
#    on next launch instead of leaving you with a broken command.
TARGET="$INSTALL_DIR/chew"
if [ -e "$TARGET" ] || [ -L "$TARGET" ]; then
    rm -f "$TARGET"
fi
cat > "$TARGET" <<'WRAPPER'
#!/usr/bin/env bash
# CHEW launcher — built by install.sh. Auto-rebuilds the binary if missing.
set -e
REPO=$(cat "$HOME/.chew-home" 2>/dev/null || true)
if [ -z "$REPO" ] || [ ! -d "$REPO" ]; then
    echo "CHEW isn't installed properly (no ~/.chew-home, or its target is gone)."
    echo "Run install.sh from inside the CHEWAgent folder."
    exit 1
fi
if [ ! -x "$REPO/chew" ]; then
    echo "Building chew (first launch since pull)..."
    if ! command -v go >/dev/null 2>&1; then
        echo "Go isn't installed. Install from https://go.dev/dl/ and try again."
        exit 1
    fi
    ( cd "$REPO" && go build -o chew ./cmd/chew/chat/repl ) || {
        echo "Build failed. Check the repo at $REPO."
        exit 1
    }
fi
exec "$REPO/chew" "$@"
WRAPPER
chmod +x "$TARGET"
green "    installed: $TARGET (auto-rebuilds binary if missing)"

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
