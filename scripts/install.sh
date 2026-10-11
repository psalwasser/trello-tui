#!/bin/sh
# Build trello-tui for this machine (macOS or Linux, detected automatically)
# and install it. Usage: ./scripts/install.sh [install-dir]
set -eu
cd "$(dirname "$0")/.."

dest=${1:-$HOME/.local/bin}

if ! command -v go >/dev/null 2>&1; then
  if command -v mise >/dev/null 2>&1; then
    exec mise exec -- "$0" "$@"
  fi
  case "$(uname -s)" in
    Darwin) echo "Go is required: brew install go" >&2 ;;
    *) echo "Go is required: install it from https://go.dev/dl or your package manager" >&2 ;;
  esac
  exit 1
fi

mkdir -p "$dest"
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$dest/trello-tui" .
echo "Installed $dest/trello-tui ($(go env GOOS)/$(go env GOARCH))"

case ":$PATH:" in
  *":$dest:"*) ;;
  *) echo "Note: $dest is not on your PATH. Add this to your shell profile:"
     echo "  export PATH=\"$dest:\$PATH\"" ;;
esac
