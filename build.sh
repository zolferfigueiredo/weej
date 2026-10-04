#!/bin/bash
# Builds build/WeeJ.exe. Pass --console for a build that keeps its console window (run.sh uses this).
set -euo pipefail
cd "$(dirname "$0")"
# shellcheck source=scripts/env.sh
. scripts/env.sh
need_go

console=0
[ "${1:-}" != --console ] || console=1

VERSION=$(app_version)
[ -n "$VERSION" ] || { echo "Couldn't read AppVersion from internal/core/version.go." >&2; exit 1; }

if is_windows; then
    "$GO" test ./...
else
    "$GO" test ./internal/core/... ./internal/lang/... ./internal/draw/...
fi

"$GO" run ./tools/mkicon winres
env -u GOOS -u GOARCH "$GO" tool go-winres make --in winres/winres.json --arch amd64 --product-version "$VERSION.0" --file-version "$VERSION.0"

ldflags="-s -w"
[ "$console" -eq 1 ] || ldflags="$ldflags -H windowsgui"

mkdir -p build
rm -f build/WeeJ.exe
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 "$GO" build -trimpath -ldflags "$ldflags" -o build/WeeJ.exe .

echo "Built: build/WeeJ.exe ($VERSION)"
[ "$console" -eq 0 ] || echo "Console build: the slider line prints in the terminal."
