#!/bin/bash
# Installs WeeJ to start at login and keeps it running. ./install.sh --uninstall removes it.
set -euo pipefail
cd "$(dirname "$0")"
# shellcheck source=scripts/env.sh
. scripts/env.sh

if ! is_windows; then
    echo "install.sh sets up WeeJ on Windows, so it needs Git Bash or WSL." >&2
    exit 1
fi

LOG='%TEMP%\weej.log'
EXE=./build/WeeJ.exe

if [ "${1:-}" = --uninstall ]; then
    [ -x "$EXE" ] || ./build.sh
    "$EXE" --quit || true
    "$EXE" --login off
    echo "Uninstalled."
    exit 0
fi

[ -x "$EXE" ] || ./build.sh
"$EXE" --quit || true

args=(--keep-alive)
if [ -n "${1:-}" ]; then
    args+=("$1")
fi
args+=(--log "$LOG")

# --keep-alive supervises the app itself: Task Scheduler can't restart a crashed app, but a clean Quit exits 0 and is left alone.
"$EXE" --login on "${args[@]}"
"$EXE" --detach "${args[@]}"

echo "Installed and running. It will start again at every login."
echo "Log: $LOG"
echo "Remove it with: ./install.sh --uninstall"
