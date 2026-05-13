#!/usr/bin/env bash
# install-macos.sh — build and install anywhere on macOS.
#
# User install (no sudo):   ./install-macos.sh             → ~/.local/bin
# System install (sudo):    sudo ./install-macos.sh         → /usr/local/bin
# Uninstall:                ./install-macos.sh --uninstall
#                           sudo ./install-macos.sh --uninstall

set -euo pipefail

# ── Output helpers ────────────────────────────────────────────────────────────

if [ -t 1 ]; then
    BOLD='\033[1m' CYAN='\033[0;36m' GREEN='\033[0;32m'
    YELLOW='\033[0;33m' RED='\033[0;31m' DIM='\033[2m' RESET='\033[0m'
else
    BOLD='' CYAN='' GREEN='' YELLOW='' RED='' DIM='' RESET=''
fi

step() { printf "  ${CYAN}→${RESET}  %s\n" "$*"; }
ok()   { printf "  ${GREEN}✓${RESET}  %s\n" "$*"; }
warn() { printf "  ${YELLOW}⚠${RESET}  %s\n" "$*" >&2; }
die()  { printf "\n  ${RED}✗${RESET}  %s\n\n" "$*" >&2; exit 1; }

DIVIDER="${DIM}────────────────────────────────────────────────${RESET}"

# ── Parse flags ───────────────────────────────────────────────────────────────

UNINSTALL=false
for arg in "$@"; do
    case "$arg" in
        --uninstall|-u) UNINSTALL=true ;;
        *) die "Unknown argument: $arg\nUsage: $0 [--uninstall]" ;;
    esac
done

# ── Resolve install paths ─────────────────────────────────────────────────────

if [ "$(id -u)" = "0" ]; then
    [ -z "${SUDO_USER:-}" ] && die "Run as the target user, or use: sudo ./install-macos.sh"
    BIN_DIR="/usr/local/bin"
    REAL_USER="$SUDO_USER"
    REAL_HOME=$(dscl . -read "/Users/$REAL_USER" NFSHomeDirectory | awk '{print $2}')
else
    BIN_DIR="$HOME/.local/bin"
    REAL_USER="$USER"
    REAL_HOME="$HOME"
fi

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLIST_LABEL="com.anywhere.daemon"
PLIST_FILE="$REAL_HOME/Library/LaunchAgents/$PLIST_LABEL.plist"
FISH_FUNC="$REAL_HOME/.config/fish/functions/any.fish"

MARK_BEGIN="# --- anywhere begin ---"
MARK_END="# --- anywhere end ---"

# ── Build ─────────────────────────────────────────────────────────────────────

do_build() {
    command -v go >/dev/null 2>&1 || die "Go not found — install from https://go.dev/dl/"

    ARCH=$(uname -m)
    case "$ARCH" in
        arm64)  GOARCH=arm64 ;;
        x86_64) GOARCH=amd64 ;;
        *)       die "Unsupported architecture: $ARCH" ;;
    esac

    step "Building anywhere (GOOS=darwin GOARCH=$GOARCH)"
    mkdir -p "$HERE/dist"
    GOOS=darwin GOARCH=$GOARCH go build -o "$HERE/dist/anywhere" "$HERE"
    ok "Built $HERE/dist/anywhere"

    step "Building anywhere-daemon (GOOS=darwin GOARCH=$GOARCH)"
    (cd "$HERE/daemon" && GOOS=darwin GOARCH=$GOARCH go build -o "$HERE/dist/anywhere-daemon" .)
    ok "Built $HERE/dist/anywhere-daemon"
}

# ── Uninstall ─────────────────────────────────────────────────────────────────

do_uninstall() {
    printf "\n${BOLD}⚡ anywhere uninstall (macOS)${RESET}\n%b\n\n" "$DIVIDER"

    # 1 — Stop and remove launchd agent
    if [ -f "$PLIST_FILE" ]; then
        step "Stopping anywhere-daemon"
        launchctl bootout "gui/$(id -u "$REAL_USER")/$PLIST_LABEL" 2>/dev/null || \
            launchctl unload "$PLIST_FILE" 2>/dev/null || true
        rm -f "$PLIST_FILE"
        ok "LaunchAgent removed"
    fi

    # 2 — Remove binaries
    local found=false
    for bin in anywhere anywhere-daemon; do
        if [ -f "$BIN_DIR/$bin" ]; then
            step "Removing $BIN_DIR/$bin"
            rm -f "$BIN_DIR/$bin"
            ok "Removed $bin"
            found=true
        fi
    done
    $found || warn "No binaries found in $BIN_DIR"

    # 3 — Remove shell integration from rc files
    for rc in \
        "$REAL_HOME/.zprofile" \
        "$REAL_HOME/.zshrc" \
        "$REAL_HOME/.bashrc" \
        "$REAL_HOME/.bash_profile" \
        "$REAL_HOME/.kshrc" \
        "$REAL_HOME/.profile"; do
        if grep -q "$MARK_BEGIN" "$rc" 2>/dev/null; then
            step "Cleaning $(basename "$rc")"
            # macOS sed requires an explicit backup extension with -i
            sed -i '' '/^# --- anywhere begin ---/,/^# --- anywhere end ---/d' "$rc"
            ok "$(basename "$rc")"
        fi
    done

    # 4 — Remove fish function
    if [ -f "$FISH_FUNC" ]; then
        step "Removing fish function"
        rm -f "$FISH_FUNC"
        ok "Removed $FISH_FUNC"
    fi

    printf "\n${GREEN}✓ anywhere uninstalled.${RESET}\n\n"
}

# ── Install ───────────────────────────────────────────────────────────────────

do_install() {
    printf "\n${BOLD}⚡ anywhere install (macOS)${RESET}\n%b\n\n" "$DIVIDER"

    # 1 — Build (or find pre-built binaries)
    local anywhere_bin="$HERE/dist/anywhere"
    local daemon_bin="$HERE/dist/anywhere-daemon"

    if [ ! -f "$anywhere_bin" ] || [ ! -f "$daemon_bin" ]; then
        do_build
    else
        step "Using existing binaries in dist/"
        ok "Skipping build"
    fi

    # 2 — Install binaries
    step "Installing to $BIN_DIR"
    mkdir -p "$BIN_DIR"
    install -m 755 "$anywhere_bin" "$BIN_DIR/anywhere"
    install -m 755 "$daemon_bin"   "$BIN_DIR/anywhere-daemon"
    ok "anywhere + anywhere-daemon → $BIN_DIR"

    case ":${PATH}:" in
        *":$BIN_DIR:"*) ;;
        *)
            # Add BIN_DIR to PATH in all login shell rc files so it survives restart.
            for rc in "$REAL_HOME/.bash_profile" "$REAL_HOME/.zprofile"; do
                [ -f "$rc" ] || continue
                if ! grep -q "$BIN_DIR" "$rc" 2>/dev/null; then
                    step "Adding $BIN_DIR to PATH in $(basename "$rc")"
                    printf '\nexport PATH="%s:$PATH"\n' "$BIN_DIR" >> "$rc"
                    ok "PATH updated in $(basename "$rc")"
                fi
            done
            warn "$BIN_DIR added to PATH — open a new terminal or source your shell config"
            ;;
    esac

    # 3 — Shell integration
    # On macOS, Terminal.app (and most GUI terminals) start a zsh login shell,
    # which reads .zprofile but NOT .zshrc.  We write to both so the function
    # works regardless of how the shell was started (login vs non-login).
    # bash/ksh fall back to their own rc files if present.
    # Ensure the most common rc files exist so the loop always writes to them.
    # bash login shells read .bash_profile; zsh login shells read .zprofile.
    touch "$REAL_HOME/.bash_profile" "$REAL_HOME/.zprofile" "$REAL_HOME/.zshrc"
    local integrated=false
    for rc in "$REAL_HOME/.bash_profile" "$REAL_HOME/.zprofile" "$REAL_HOME/.zshrc" "$REAL_HOME/.bashrc" "$REAL_HOME/.kshrc"; do
        [ -f "$rc" ] || continue
        if grep -q "$MARK_BEGIN" "$rc" 2>/dev/null; then
            ok "$(basename "$rc") (already present)"
        else
            step "Adding 'any' function to $(basename "$rc")"
            cat >> "$rc" << 'SHELL_BLOCK'

# --- anywhere begin ---
function any {
    local _tmp
    _tmp=$(mktemp)
    anywhere --cd-file "$_tmp" "$@"
    if [ -s "$_tmp" ]; then
        cd "$(cat "$_tmp")" || true
    fi
    rm -f "$_tmp"
}
# --- anywhere end ---
SHELL_BLOCK
            ok "$(basename "$rc")"
        fi
        integrated=true
    done
    $integrated || warn "No .zshrc/.bashrc found — add the 'any' function manually (see anywhere-cd.sh)"

    # 4 — Fish integration
    if command -v fish >/dev/null 2>&1 || [ -d "$REAL_HOME/.config/fish" ]; then
        step "Installing fish function"
        mkdir -p "$(dirname "$FISH_FUNC")"
        if [ -f "$HERE/anywhere-cd.fish" ]; then
            cp "$HERE/anywhere-cd.fish" "$FISH_FUNC"
            ok "any.fish → $FISH_FUNC"
        else
            warn "anywhere-cd.fish not found — copy it manually to $FISH_FUNC"
        fi
    fi

    # 5 — LaunchAgent for anywhere-daemon
    step "Installing LaunchAgent"
    mkdir -p "$(dirname "$PLIST_FILE")"
    cat > "$PLIST_FILE" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$PLIST_LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>$BIN_DIR/anywhere-daemon</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>SoftResourceLimits</key>
  <dict>
    <key>NumberOfFiles</key>
    <integer>65536</integer>
  </dict>
  <key>HardResourceLimits</key>
  <dict>
    <key>NumberOfFiles</key>
    <integer>65536</integer>
  </dict>
  <key>StandardOutPath</key>
  <string>/tmp/anywhere-daemon.log</string>
  <key>StandardErrorPath</key>
  <string>/tmp/anywhere-daemon.log</string>
</dict>
</plist>
EOF

    # Load it for the current session (works without logout/login).
    if launchctl bootstrap "gui/$(id -u "$REAL_USER")" "$PLIST_FILE" 2>/dev/null ||
       launchctl load "$PLIST_FILE" 2>/dev/null; then
        ok "LaunchAgent loaded and started"
    else
        warn "Could not load LaunchAgent — run: launchctl load $PLIST_FILE"
    fi

    # ── Activate in current shell ─────────────────────────────────────────────
    # The 'any' function was written to ~/.zshrc but won't be available until
    # the shell re-reads it.  Source it now so the user doesn't have to open a
    # new terminal.
    # shellcheck disable=SC1090
    # We intentionally print the source command so the user can run it in their
    # own shell — this subshell can't affect the parent shell's environment.
    printf "\n  ${YELLOW}⚠${RESET}  Run this to activate in your current terminal:\n"
    printf "     ${BOLD}source ~/.zprofile${RESET}   ${DIM}# zsh${RESET}\n"
    printf "     ${BOLD}source ~/.bash_profile${RESET}  ${DIM}# bash${RESET}\n"

    # ── Summary ───────────────────────────────────────────────────────────────
    printf "\n${GREEN}✓ anywhere installed!${RESET}\n\n"
    printf "  After sourcing, use:\n"
    printf "    ${BOLD}any${RESET}       — search + Ctrl+D to cd\n"
    printf "    ${BOLD}anywhere${RESET}  — standalone (no cd)\n"
    printf "\n"
    printf "  Daemon logs:   tail -f /tmp/anywhere-daemon.log\n"
    printf "  Stop daemon:   launchctl stop %s\n" "$PLIST_LABEL"
    printf "  Start daemon:  launchctl start %s\n" "$PLIST_LABEL"
    printf "\n"
    printf "  To uninstall:\n"
    printf "    %s/install-macos.sh --uninstall\n\n" "$HERE"
}

# ── Entry point ───────────────────────────────────────────────────────────────

if $UNINSTALL; then
    do_uninstall
else
    do_install
fi
