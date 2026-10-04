#!/bin/bash
# Builds WeeJ and runs it here, quitting any running copy first so the new build takes over.
set -euo pipefail
cd "$(dirname "$0")"
# shellcheck source=scripts/env.sh
. scripts/env.sh

if ! is_windows; then
    echo "run.sh runs WeeJ, so it needs Windows (Git Bash or WSL). ./build.sh works anywhere." >&2
    exit 1
fi

# core.hooksPath is local config and does not survive a clone, so point it at the versioned
# hooks (.githooks/pre-commit blocks commits to main) the first time anyone runs this.
if [ -d .git ] && [ -d .githooks ] && [ "$(git config --get core.hooksPath || true)" != ".githooks" ]; then
    git config core.hooksPath .githooks
    echo "Enabled the repository git hooks."
fi

# --quit asks a running copy to exit and waits for it, so build.sh can overwrite the binary.
if [ -f build/WeeJ.exe ]; then
    ./build/WeeJ.exe --quit || true
fi

./build.sh --console
exec ./build/WeeJ.exe "$@"
