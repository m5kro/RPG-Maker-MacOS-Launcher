#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
mkdir -p build/helpers/arm64 build/helpers/x64 build/helpers/universal2
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -o build/helpers/arm64/nwjs-helper ./cmd/nwjs-helper
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -o build/helpers/x64/nwjs-helper ./cmd/nwjs-helper
lipo -create build/helpers/arm64/nwjs-helper build/helpers/x64/nwjs-helper -output build/helpers/universal2/nwjs-helper
lipo build/helpers/arm64/nwjs-helper -verify_arch arm64
lipo build/helpers/x64/nwjs-helper -verify_arch x86_64
lipo build/helpers/universal2/nwjs-helper -verify_arch arm64 x86_64
printf '%s\n' 'Built and checked NW.js helpers in build/helpers/{arm64,x64,universal2}/nwjs-helper'
