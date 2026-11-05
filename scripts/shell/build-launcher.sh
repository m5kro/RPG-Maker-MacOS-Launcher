#!/bin/sh
# Go + the macOS SDK only. No Python, Node, npm or Wails CLI.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)
OUT="$ROOT/build/validation/python-zero/gui"
MODE=${1:-production}
case "$MODE" in
  production) TAGS=production; NAME="RPG Maker Launcher.app" ;;
  smoke) TAGS=production,smoke; NAME="RPG Maker Launcher Smoke.app" ;;
  *) printf '%s\n' 'usage: build-launcher.sh [production|smoke] [output-name.app]' >&2; exit 2 ;;
esac
if [ "$#" -gt 2 ]; then exit 2; fi
NAME=${2:-$NAME}
case "$NAME" in */*|.*|"") printf '%s\n' 'output must be a plain .app name' >&2; exit 2 ;; *.app) ;; *) exit 2 ;; esac
if [ "$(uname -s)" != Darwin ] || [ "$(uname -m)" != arm64 ]; then
  printf '%s\n' 'native macOS arm64 is required' >&2; exit 1
fi
mkdir -p "$OUT"
if [ -e "$OUT/$NAME" ]; then printf 'refusing to replace existing output: %s\n' "$OUT/$NAME" >&2; exit 1; fi
STAGE=$(mktemp -d "$OUT/.launcher-build.XXXXXX")
trap 'rm -rf "$STAGE"' EXIT HUP INT TERM
APP="$STAGE/$NAME"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources/webui"
# 13.3 is the SDK/deployment target, not a claim of testing on macOS 13.3.
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go -C "$ROOT/src/go" build \
  -tags "$TAGS" -trimpath -ldflags='-extldflags=-mmacosx-version-min=13.3' \
  -o "$APP/Contents/MacOS/launcher" ./cmd/launcher
mkdir -p "$APP/Contents/Resources/licenses"
cp "$ROOT/LICENSE" "$APP/Contents/Resources/licenses/Launcher.txt"
GOLICENSE="$(go env GOROOT)/LICENSE"
# Homebrew keeps this one directory above libexec.
if [ ! -f "$GOLICENSE" ]; then GOLICENSE="$(go env GOROOT)/../LICENSE"; fi
cp "$GOLICENSE" "$APP/Contents/Resources/licenses/Go.txt"
WAILS=$(go -C "$ROOT/src/go" list -m -f '{{.Dir}}' github.com/wailsapp/wails/v3)
cp "$WAILS/LICENSE" "$APP/Contents/Resources/licenses/Wails.txt"
cp "$ROOT/packaging/macos/arm64/Info.plist" "$APP/Contents/Info.plist"
cp "$ROOT/assets/icons/icon.icns" "$APP/Contents/Resources/icon.icns"
cp "$ROOT/packaging/macos/arm64/EasyRPG-Standalone" "$APP/Contents/Resources/EasyRPG-Standalone"
for FILE in bg.js disable-child.js disable-net.js; do cp "$ROOT/src/javascript/$FILE" "$APP/Contents/Resources/$FILE"; done
for FILE in index.html app.js style.css; do cp "$ROOT/src/webui/$FILE" "$APP/Contents/Resources/webui/$FILE"; done
/usr/bin/plutil -lint "$APP/Contents/Info.plist"
/usr/bin/codesign --force --sign - "$APP"
/usr/bin/codesign --verify --strict "$APP"
for FILE in index.html app.js style.css; do cmp "$ROOT/src/webui/$FILE" "$APP/Contents/Resources/webui/$FILE"; done
for FILE in bg.js disable-child.js disable-net.js; do cmp "$ROOT/src/javascript/$FILE" "$APP/Contents/Resources/$FILE"; done
cmp "$ROOT/assets/icons/icon.icns" "$APP/Contents/Resources/icon.icns"
cmp "$ROOT/packaging/macos/arm64/EasyRPG-Standalone" "$APP/Contents/Resources/EasyRPG-Standalone"
mv "$APP" "$OUT/$NAME"
printf '%s\n' "$OUT/$NAME"
