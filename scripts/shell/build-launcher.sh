#!/bin/sh
# Go + the macOS SDK only. No Python, Node, npm or Wails CLI.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)
OUT="$ROOT/build/validation/python-zero/gui"
MODE=${1:-production}
case "$MODE" in
  production) TAGS=production; NAME="RPG Maker Launcher.app" ;;
  smoke) TAGS=production,smoke; NAME="RPG Maker Launcher Smoke.app" ;;
  *) printf '%s\n' 'usage: build-launcher.sh [production|smoke] [output-name.app] [absolute-output-directory]' >&2; exit 2 ;;
esac
if [ "$#" -gt 3 ]; then exit 2; fi
NAME=${2-$NAME}
OUT=${3-$OUT}
case "$NAME" in */*|*\\*|.*|"") printf '%s\n' 'output must be a plain .app name' >&2; exit 2 ;; *.app) ;; *) exit 2 ;; esac
if [ "$(printf '%s' "$NAME$OUT" | LC_ALL=C tr -d '[:cntrl:]')" != "$NAME$OUT" ]; then
  printf '%s\n' 'control characters in output path' >&2; exit 2
fi
case "$OUT" in /*) ;; *) printf '%s\n' 'output directory must be absolute' >&2; exit 2 ;; esac
case "$OUT/" in *//*|*/./*|*/../*) printf '%s\n' 'output directory must be a clean physical path' >&2; exit 2 ;; esac
check_output() {
  CHECK="$OUT/$NAME"
  while [ "$CHECK" != / ]; do
    if [ -L "$CHECK" ]; then printf 'symlink output/parent refused: %s\n' "$CHECK" >&2; return 1; fi
    CHECK=$(dirname -- "$CHECK")
  done
}
check_output
DEST="$OUT/$NAME"
REPLACE=false
if [ "$MODE" = production ] && [ "$DEST" = "$ROOT/dist/RPG-Maker-Launcher.app" ]; then REPLACE=true; fi
if [ -e "$DEST" ] && { [ "$REPLACE" != true ] || [ ! -d "$DEST" ]; }; then
  printf 'refusing to replace existing output: %s\n' "$DEST" >&2; exit 1
fi
if [ "$(uname -s)" != Darwin ] || [ "$(uname -m)" != arm64 ]; then
  printf '%s\n' 'native macOS arm64 is required' >&2; exit 1
fi
mkdir -p "$OUT"
# Serialize cooperating builds, including the gap between the two renames.
LOCK="$OUT/.launcher-build.lock"
if ! mkdir "$LOCK"; then printf 'another build or stale lock: %s\n' "$LOCK" >&2; exit 1; fi
STAGE=
BACKUP=
COMMITTED=false
cleanup() {
  STATUS=$?
  trap - EXIT HUP INT TERM
  if [ "$COMMITTED" != true ] && [ -n "$BACKUP" ] && [ -d "$BACKUP/$NAME" ]; then
    if [ ! -e "$DEST" ] && [ ! -L "$DEST" ] && mv -n "$BACKUP/$NAME" "$OUT/" && [ ! -e "$BACKUP/$NAME" ]; then
      rmdir "$BACKUP"
    else
      printf 'restore failed; recovery bundle preserved: %s\n' "$BACKUP/$NAME" >&2
      STATUS=1
    fi
  elif [ -n "$BACKUP" ]; then
    if [ "$COMMITTED" = true ]; then rm -rf "$BACKUP"; else rmdir "$BACKUP"; fi
  fi
  if [ -n "$STAGE" ]; then rm -rf "$STAGE"; fi
  rmdir "$LOCK"
  exit "$STATUS"
}
trap cleanup EXIT
trap 'exit 130' HUP INT TERM
STAGE=$(mktemp -d "$OUT/.launcher-build.XXXXXX")
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
test "$(/usr/bin/lipo -archs "$APP/Contents/MacOS/launcher")" = arm64
/usr/bin/plutil -lint "$APP/Contents/Info.plist"
/usr/bin/codesign --force --sign - "$APP"
/usr/bin/codesign --verify --strict "$APP"
for FILE in index.html app.js style.css; do cmp "$ROOT/src/webui/$FILE" "$APP/Contents/Resources/webui/$FILE"; done
for FILE in bg.js disable-child.js disable-net.js; do cmp "$ROOT/src/javascript/$FILE" "$APP/Contents/Resources/$FILE"; done
cmp "$ROOT/assets/icons/icon.icns" "$APP/Contents/Resources/icon.icns"
cmp "$ROOT/packaging/macos/arm64/EasyRPG-Standalone" "$APP/Contents/Resources/EasyRPG-Standalone"
check_output
if [ -e "$DEST" ]; then
  if [ "$REPLACE" != true ] || [ ! -d "$DEST" ]; then
    printf 'output appeared during build: %s\n' "$DEST" >&2; exit 1
  fi
  BACKUP=$(mktemp -d "$OUT/.launcher-backup.XXXXXX")
  mv "$DEST" "$BACKUP/$NAME"
fi
# Target the parent, not DEST: an unexpected directory must never receive a nested app.
mv -n "$APP" "$OUT/"
if [ -e "$APP" ]; then printf 'output appeared during publish: %s\n' "$DEST" >&2; exit 1; fi
COMMITTED=true
printf '%s\n' "$DEST"
