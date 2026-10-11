#!/bin/sh
# Cross-compile trello-tui for macOS and Linux into dist/.
set -eu
cd "$(dirname "$0")/.."
mkdir -p dist
for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64; do
  os=${target%/*}
  arch=${target#*/}
  out="dist/trello-tui-$os-$arch"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags="-s -w" -o "$out" .
  echo "built $out"
done
