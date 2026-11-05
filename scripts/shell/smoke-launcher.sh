#!/bin/sh
# Opt-in native WebKit acceptance. Isolation is not an OS sandbox.
set -eu
REPO=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)
if [ "$#" -ne 3 ] || [ "$3" != --reviewed-runtime ]; then
  printf '%s\n' 'usage: smoke-launcher.sh ABS_NEW_ROOT ABS_RUNTIME/v0.94.1 --reviewed-runtime' >&2
  exit 2
fi
WORK=$1
RUNTIME=$2
for P in "$WORK" "$RUNTIME"; do
  case "$P" in /*) ;; *) printf '%s\n' 'absolute physical paths required' >&2; exit 2 ;; esac
  case "$P/" in *//*|*/./*|*/../*) exit 2 ;; esac
  if [ "$(printf '%s' "$P" | LC_ALL=C tr -d '[:cntrl:]')" != "$P" ]; then exit 2; fi
  CHECK=$P
  while [ "$CHECK" != / ]; do
    if [ -L "$CHECK" ]; then printf 'symlink path refused: %s\n' "$CHECK" >&2; exit 2; fi
    CHECK=$(dirname -- "$CHECK")
  done
done
if [ -e "$WORK" ] || [ ! -d "$(dirname -- "$WORK")" ]; then
  printf '%s\n' 'root must not exist; its physical parent must exist' >&2; exit 2
fi
if [ "$(basename -- "$RUNTIME")" != v0.94.1 ] || [ ! -x "$RUNTIME/nwjs.app/Contents/MacOS/nwjs" ]; then
  printf '%s\n' 'existing reviewed v0.94.1 runtime required (no download)' >&2; exit 2
fi
case "$WORK/" in "$RUNTIME/"*) printf '%s\n' 'root must be outside the runtime' >&2; exit 2 ;; esac
umask 077
mkdir "$WORK"
mkdir -p "$WORK/home/Downloads" "$WORK/root" "$WORK/tmp" "$WORK/cache"
printf 'owned\n' > "$WORK/root/.gui-smoke-owned"
# Clone/copy only; never sign or modify the external runtime.
/bin/cp -cpR "$RUNTIME" "$WORK/root/v0.94.1"
sh "$REPO/scripts/shell/build-launcher.sh" smoke Smoke.app "$WORK"
export HOME="$WORK/home" CFFIXED_USER_HOME="$WORK/home" TMPDIR="$WORK/tmp" XDG_CACHE_HOME="$WORK/cache"
export RPGM_GUI_SMOKE_RESULT="$WORK/result.json"
cd /
exec "$WORK/Smoke.app/Contents/MacOS/launcher" --root "$WORK/root"
