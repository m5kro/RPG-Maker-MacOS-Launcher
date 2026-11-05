#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
mkdir -p "$ROOT/build/helpers/arm64"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go -C "$ROOT/src/go" build -trimpath -o "$ROOT/build/helpers/arm64/nwjs-helper" ./cmd/nwjs-helper
test "$(lipo -archs "$ROOT/build/helpers/arm64/nwjs-helper")" = arm64
printf '%s\n' 'Built and checked arm64 NW.js helper: build/helpers/arm64/nwjs-helper'
