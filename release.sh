#!/bin/bash
# Set AppVersion in internal/core/version.go first. Tags the current commit and pushes it;
# .github/workflows/release.yml builds the exe, zip and installer and publishes the GitHub release.
set -euo pipefail
cd "$(dirname "$0")"
# shellcheck source=scripts/env.sh
. scripts/env.sh

VERSION=$(app_version)
[ -n "$VERSION" ] || { echo "Couldn't read AppVersion from internal/core/version.go." >&2; exit 1; }

branch=$(git symbolic-ref --short HEAD 2>/dev/null || true)
if [ "$branch" != main ]; then
    echo "Switch to main first." >&2
    exit 1
fi

if ! git diff --quiet || ! git diff --cached --quiet; then
    echo "Commit or stash your changes first." >&2
    exit 1
fi

git fetch --quiet origin main
if [ "$(git rev-parse HEAD)" != "$(git rev-parse origin/main)" ]; then
    echo "Push or pull main first." >&2
    exit 1
fi

tag="v$VERSION"
if git rev-parse -q --verify "refs/tags/$tag" >/dev/null || git ls-remote --exit-code origin "refs/tags/$tag" >/dev/null 2>&1; then
    echo "$tag already exists. Raise AppVersion in internal/core/version.go first." >&2
    exit 1
fi

git tag -a "$tag" -m "WeeJ $VERSION"
git push origin "$tag"

echo "Pushed $tag. GitHub Actions builds and publishes the release:"
echo "https://github.com/zolferfigueiredo/weej/actions"

[ "${1:-}" = --url ] || exit 0
url="https://github.com/zolferfigueiredo/weej/releases/download/$tag/WeeJ-$VERSION-x64-setup.exe"
loc=$(curl -fsS -o /dev/null -w '%{redirect_url}' --data-urlencode "url=$url" https://url.zolfer.com/dmg)
echo "Download link: https://url.zolfer.com/${loc##*c=}"
