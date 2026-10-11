#!/bin/sh
# Build trello-tui binaries for the supported platforms into dist/:
# Apple Silicon Macs and x86_64 Linux.
set -eu
cd "$(dirname "$0")/.."
mkdir -p dist
for target in darwin/arm64 linux/amd64; do
  os=${target%/*}
  arch=${target#*/}
  out="dist/trello-tui-$os-$arch"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags="-s -w" -o "$out" .
  echo "built $out"
done
