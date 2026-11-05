#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
mkdir -p build/helpers/arm64
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -o build/helpers/arm64/nwjs-helper ./cmd/nwjs-helper
test "$(lipo -archs build/helpers/arm64/nwjs-helper)" = arm64
printf '%s\n' 'Built and checked arm64 NW.js helper: build/helpers/arm64/nwjs-helper'
