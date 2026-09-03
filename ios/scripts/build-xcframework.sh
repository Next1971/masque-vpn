#!/usr/bin/env bash
# Build Mobile.xcframework from the shared Go core (gomobile). Requires Xcode + Go.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
GOSRC="$ROOT/android/go-src/masque-vpn"
OUT="$ROOT/ios/Frameworks/Mobile.xcframework"

if ! command -v go >/dev/null; then
  echo "Go is not in PATH" >&2
  exit 1
fi
if ! command -v xcodebuild >/dev/null; then
  echo "xcodebuild not found; this script must run on macOS with Xcode" >&2
  exit 1
fi

go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest
export PATH="$(go env GOPATH)/bin:$PATH"
gomobile init

mkdir -p "$(dirname "$OUT")"
rm -rf "$OUT"

pushd "$GOSRC" >/dev/null
gomobile bind -target=ios,iossimulator -iosversion=16.0 -o "$OUT" ./mobile
popd >/dev/null

echo "DONE: $OUT"
