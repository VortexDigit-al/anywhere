#!/usr/bin/env bash
# anywhere-cd.sh — bash / zsh / ksh integration for anywhere.
#
# Covers: bash, zsh, ksh (any POSIX-compatible shell).
# Fish users: see anywhere-cd.fish instead.
#
# Usage
# -----
#   Source this file from within a shell function so that the cd propagates
#   to the calling shell.  Direct execution won't work because a subprocess
#   cannot change the parent's directory.
#
# Shell integration
# -----------------
#   bash — add to ~/.bashrc:
#   zsh  — add to ~/.zshrc:
#   ksh  — add to ~/.kshrc:
#
#   function any {
#       local _tmp
#       _tmp=$(mktemp)
#       anywhere --cd-file "$_tmp" "$@"
#       if [ -s "$_tmp" ]; then
#           cd "$(cat "$_tmp")" || true
#       fi
#       rm -f "$_tmp"
#   }
#
#   fish — copy anywhere-cd.fish to ~/.config/fish/functions/any.fish
#
# How it works
# ------------
# anywhere is launched with --cd-file pointing to a temp file.
# When you press Ctrl+D on a result, anywhere writes that directory to the
# file and exits.  The wrapper reads the file and calls cd.
# Without the wrapper, Ctrl+D opens a new terminal window at the directory
# instead — useful, but cannot change *this* shell's directory.

_anywhere_tmp=$(mktemp)
anywhere --cd-file "$_anywhere_tmp" "$@"
if [ -s "$_anywhere_tmp" ]; then
    cd "$(cat "$_anywhere_tmp")" || true
fi
rm -f "$_anywhere_tmp"
unset _anywhere_tmp
