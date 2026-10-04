# shellcheck shell=bash
# Detects the environment (Git Bash, WSL, or other) and exposes WEEJ_ENV, GO, and helper functions.

case "$(uname -s)" in
    MINGW*|MSYS*|CYGWIN*)
        WEEJ_ENV=gitbash
        GO=go
        ;;
    *)
        if grep -qi microsoft /proc/version 2>/dev/null; then
            WEEJ_ENV=wsl
            GO=go.exe
            case "$PWD" in
                /mnt/*) ;;
                *)
                    echo "In WSL, keep the repo on a Windows drive (/mnt/c/...), because the Windows Go builds it." >&2
                    exit 1
                    ;;
            esac
            export WSLENV="${WSLENV:+$WSLENV:}GOOS/w:GOARCH/w:CGO_ENABLED/w"
        else
            WEEJ_ENV=other
            GO=go
        fi
        ;;
esac

is_windows() {
    [ "$WEEJ_ENV" = gitbash ] || [ "$WEEJ_ENV" = wsl ]
}

need_go() {
    if ! command -v "$GO" >/dev/null 2>&1; then
        echo "Go isn't installed. On Windows: winget install GoLang.Go (then open a new terminal)." >&2
        exit 1
    fi
}

app_version() {
    sed -n 's/^const AppVersion = "\(.*\)"$/\1/p' internal/core/version.go
}
