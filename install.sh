#!/usr/bin/env bash
# install.sh — install or uninstall anywhere on Linux.
#
# User install (no sudo):   ./install.sh             → ~/.local/bin
# System install (sudo):    sudo ./install.sh         → /usr/local/bin
# Uninstall:                ./install.sh --uninstall
#                           sudo ./install.sh --uninstall

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

# ── Helpers ───────────────────────────────────────────────────────────────────

# Run a command as the pre-sudo user with the systemd/dbus env vars it needs.
run_as_user() {
    if [ "$(id -u)" = "0" ] && [ -n "${SUDO_USER:-}" ]; then
        local uid; uid=$(id -u "$SUDO_USER")
        sudo -u "$SUDO_USER" \
            XDG_RUNTIME_DIR="/run/user/$uid" \
            DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/$uid/bus" \
            "$@"
    else
        "$@"
    fi
}

# Locate an executable by name, checking next to this script and in dist/.
find_bin() {
    local name="$1"
    local here; here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    for dir in "$here" "$here/dist" "$(pwd)" "$(pwd)/dist"; do
        if [ -f "$dir/$name" ] && [ -x "$dir/$name" ]; then
            echo "$dir/$name"; return 0
        fi
    done
    return 1
}

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
    [ -z "${SUDO_USER:-}" ] && die "Run as the target user, or use: sudo -u <user> $0"
    BIN_DIR="/usr/local/bin"
    REAL_USER="$SUDO_USER"
    REAL_HOME="$(getent passwd "$REAL_USER" | cut -d: -f6)"
else
    BIN_DIR="$HOME/.local/bin"
    REAL_USER="$USER"
    REAL_HOME="$HOME"
fi

SERVICE_FILE="$REAL_HOME/.config/systemd/user/anywhere-daemon.service"
FISH_FUNC="$REAL_HOME/.config/fish/functions/any.fish"

# Sentinel lines that bracket the shell function in POSIX rc files.
MARK_BEGIN="# --- anywhere begin ---"
MARK_END="# --- anywhere end ---"

# ── Uninstall ─────────────────────────────────────────────────────────────────

do_uninstall() {
    printf "\n${BOLD}⚡ anywhere uninstall${RESET}\n%b\n\n" "$DIVIDER"

    # 1 — Stop and remove systemd service
    if [ -f "$SERVICE_FILE" ]; then
        step "Stopping anywhere-daemon"
        run_as_user systemctl --user stop    anywhere-daemon 2>/dev/null || true
        run_as_user systemctl --user disable anywhere-daemon 2>/dev/null || true
        rm -f "$SERVICE_FILE"
        run_as_user systemctl --user daemon-reload 2>/dev/null || true
        ok "Service removed"
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

    # 3 — Remove POSIX shell integration from any rc file it was added to
    for rc in \
        "$REAL_HOME/.bashrc" \
        "$REAL_HOME/.bash_profile" \
        "$REAL_HOME/.zshrc" \
        "$REAL_HOME/.kshrc" \
        "$REAL_HOME/.profile"; do
        if grep -q "$MARK_BEGIN" "$rc" 2>/dev/null; then
            step "Cleaning $(basename "$rc")"
            sed -i '/^# --- anywhere begin ---/,/^# --- anywhere end ---/d' "$rc"
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
    printf "\n${BOLD}⚡ anywhere install${RESET}\n%b\n\n" "$DIVIDER"

    # 1 — Locate source binaries (built by build.ps1 / go build)
    step "Locating binaries"
    local anywhere_src daemon_src
    anywhere_src=$(find_bin anywhere) \
        || die "anywhere binary not found — build it first:\n    GOOS=linux GOARCH=amd64 go build -o dist/anywhere ."
    daemon_src=$(find_bin anywhere-daemon) \
        || die "anywhere-daemon not found — build it first:\n    GOOS=linux GOARCH=amd64 go build -o dist/anywhere-daemon ./daemon"
    ok "Found binaries"

    # 2 — Copy binaries
    step "Installing to $BIN_DIR"
    mkdir -p "$BIN_DIR"
    install -m 755 "$anywhere_src" "$BIN_DIR/anywhere"
    install -m 755 "$daemon_src"   "$BIN_DIR/anywhere-daemon"
    ok "anywhere + anywhere-daemon → $BIN_DIR"

    case ":${PATH}:" in
        *":$BIN_DIR:"*) ;;
        *) warn "$BIN_DIR is not in \$PATH — open a new terminal or add it to your shell config" ;;
    esac

    # 3 — POSIX shell integration (bash / zsh / ksh)
    local integrated=false
    for rc in "$REAL_HOME/.bashrc" "$REAL_HOME/.zshrc" "$REAL_HOME/.kshrc"; do
        [ -f "$rc" ] || continue
        if grep -q "$MARK_BEGIN" "$rc" 2>/dev/null; then
            ok "$(basename "$rc") (already present)"
        else
            step "Adding 'any' function to $(basename "$rc")"
            # Single-quoted heredoc — no escaping needed inside the block.
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
    $integrated || warn "No .bashrc/.zshrc found — add the 'any' function manually (see anywhere-cd.sh)"

    # 4 — Fish integration
    local here; here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    if command -v fish > /dev/null 2>&1 || [ -d "$REAL_HOME/.config/fish" ]; then
        step "Installing fish function"
        mkdir -p "$(dirname "$FISH_FUNC")"
        if [ -f "$here/anywhere-cd.fish" ]; then
            cp "$here/anywhere-cd.fish" "$FISH_FUNC"
            ok "any.fish → $FISH_FUNC"
        else
            warn "anywhere-cd.fish not found next to install.sh — copy it manually to $FISH_FUNC"
        fi
    fi

    # 5 — Systemd user service
    step "Installing systemd user service"
    if ! command -v systemctl > /dev/null 2>&1; then
        warn "systemctl not found — skipping service setup"
        warn "Start the daemon manually: anywhere-daemon &"
    else
        mkdir -p "$(dirname "$SERVICE_FILE")"
        # ExecStart is expanded here by bash so the literal path ends up in the file.
        cat > "$SERVICE_FILE" << EOF
[Unit]
Description=anywhere file-index daemon
After=default.target

[Service]
ExecStart=$BIN_DIR/anywhere-daemon
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF
        # Lingering lets user services start at boot without an active login session.
        loginctl enable-linger "$REAL_USER" 2>/dev/null \
            || warn "Could not enable lingering — daemon won't autostart at boot while logged out"

        run_as_user systemctl --user daemon-reload

        if run_as_user systemctl --user enable --now anywhere-daemon 2>/dev/null; then
            ok "Service enabled and started"
        else
            warn "Could not start service — run: systemctl --user enable --now anywhere-daemon"
        fi
    fi

    # ── Summary ───────────────────────────────────────────────────────────────
    printf "\n${GREEN}✓ anywhere installed!${RESET}\n\n"
    printf "  Open a new terminal, then:\n"
    printf "    ${BOLD}any${RESET}       — search + Ctrl+D to cd\n"
    printf "    ${BOLD}anywhere${RESET}  — standalone (no cd)\n"
    printf "\n"
    printf "  To uninstall:\n"
    printf "    %s --uninstall\n\n" "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/install.sh"
}

# ── Entry point ───────────────────────────────────────────────────────────────

if $UNINSTALL; then
    do_uninstall
else
    do_install
fi
