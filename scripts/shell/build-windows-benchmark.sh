#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
NAME=MZ-Benchmark-Windows-x64
mkdir -p "$ROOT/build" "$ROOT/dist"
STAGE=$(mktemp -d "$ROOT/build/windows-benchmark.XXXXXXXX")
trap 'rm -rf "$STAGE"' EXIT HUP INT TERM
PACKAGE="$STAGE/$NAME"
OUTPUT="$ROOT/dist/$NAME-$(date +%Y%m%d-%H%M%S).zip"
if test -e "$OUTPUT"; then
    printf '%s\n' "Refusing to overwrite: $OUTPUT" >&2
    exit 1
fi
mkdir -p "$PACKAGE/source/cmd/benchmark-mz"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go -C "$ROOT/src/go" build -trimpath -o "$PACKAGE/benchmark-mz.exe" ./cmd/benchmark-mz
file "$PACKAGE/benchmark-mz.exe" | grep 'PE32+.*x86-64'
cp "$ROOT/scripts/windows/Start-Benchmark.cmd" "$ROOT/scripts/windows/Run-Benchmark.ps1" "$PACKAGE/"
cp "$ROOT/docs/validation/windows-benchmark.md" "$PACKAGE/README.txt"
cp "$ROOT/LICENSE" "$PACKAGE/LICENSE"
cp "$ROOT/src/go/go.mod" "$ROOT/src/go/go.sum" "$PACKAGE/source/"
cp "$ROOT/src/go/cmd/benchmark-mz/"*.go "$ROOT/src/go/cmd/benchmark-mz/"*.js "$PACKAGE/source/cmd/benchmark-mz/"
(cd "$STAGE" && zip -qr "$OUTPUT" "$NAME")
printf '\nCreated: %s\n' "$OUTPUT"
shasum -a 256 "$OUTPUT"
