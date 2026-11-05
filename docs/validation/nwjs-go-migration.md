# Incremental Go migration: real MZ validation

> ARM専用化の現状注記：ランチャー／Go helperはmacOS Apple Silicon arm64のみ対応します。NW.js一覧もarm64配布のある版のみで、旧`--arch`・`--rosetta`は拒否します。本文の320版・旧CLI・Rosetta同意・3アーキテクチャなどは過去の事実として残し、現在の再実行手順は末尾「Reproduce safely」に分けています。ARM専用化でこの実ゲーム試験を再実行したわけではありません。

> 現状注記：この記録は機能撤去前の検証です。現在はチート用JS/CSSの注入・除去、`game_en.exe`からの英語版展開、外部セーブ編集サイトへの導線を撤去しています。以下のテスト件数・条件・結果は当時の履歴であり、撤去後の実ゲーム再試験を示すものではありません。通常のゲーム内セーブ・ロード試験の記録はそのまま残しています。

## ARM-only regression checks (current; separate from the historical game run)

- TDD evidence: `build/validation/arm-only/python-red-complete.log`, `safety-red.log`, `external-red.log`, `distribution-red.log`; final `python-green.log`: **58 Python/Qt tests passed, no skips**. The first whole-suite RED attempt timed out on the obsolete Rosetta dialog; the completed RED run mocks the dialog closed. `python-green-attempt.log` separately records the old built helper's CLI mismatch during migration, not a retained Python failure.
- Main executable checks cover arm64 thin and external Universal/FAT32/FAT64 (both byte orders), malformed/truncated/range-invalid input, Intel-only/missing/non-executable/symlink/FIFO rejection, and old NW.js receipt/marker preservation. This is not a complete Mach-O loader, dylib audit or signature validation.
- ARM helper build and arm64 PyInstaller build passed. New packaging outputs are under `build/validation/arm-only/dist/`, not a cleanup of previous `dist/` or x64/Universal2 build artifacts. `packaged-smoke.json` records native main/helper checks, a read-only check of the previously installed v0.94.1, and a 3-second offscreen startup in a fresh isolated HOME. `packaged-codesign.log` records local ad-hoc strict verification success; no Developer ID/notarization/Finder/Gatekeeper claim.
- No new external game/runtime download or real gameplay retest was performed for these Python/distribution checks. External installer tests use header-only Mach-O ZIP fixtures; they do not execute them. The official EasyRPG CI URL and MKXP-Z universal URL remain unchanged, but their downloaded main executables must now contain arm64 before replacement. Failed download/validation preserves the old app; failed rollback preserves `.AppName.previous` outside staging for manual recovery.

## Result and boundary (historical Go migration)

**Passed for one real MZ fixture on Apple Silicon:** official runtime installation by the new Go helper, a real NW.js renderer, keyboard-driven title → New Game → movement/event → Save → process close/relaunch → Continue/Load, and restored state. Separately, the production Python window/QProcess integration passed against the real helper and an isolated copy of that installation.

This is **not** an all-games compatibility claim, a complete Go rewrite, a distribution-signing check, or an end-to-end test of the launcher's game-patching/start/export path. The game was launched directly from the helper-installed runtime without launcher patches. No core implementation was edited by this validation task.

### Environment and evidence

- macOS **26.5.1 (25F80)**, Darwin 25.5.0, **Apple M1 Ultra / arm64**, 128 GiB RAM.
- Go **1.27.3**; Python **3.11.2**; Qt/PySide6 **6.12.0**.
- NW.js SDK **0.94.1**, Chromium **131.0.6778.109**, Node **23.3.0**; engine **MZ 1.8.0**.
- Host clock recorded **2026-11-05 UTC**. The evidence directory's `mz-20260717` component is only a run label, not a reliable execution date.
- Evidence root: `build/validation/mz-20260717/evidence/` (ignored, local only).
- Final renderer evidence: `renderer-1793850439494025000/`. Nine PNG screenshots, per-stage state JSON, CDP commands, launch arguments/environment/PIDs, listener checks, runtime versions and stderr are preserved there.
- `final-evidence.json` records source SHA-256s, environment, unchanged fixture files and closed processes/ports. `download-summary.json`, `install.stdout`, `install.stderr`, manifest/HTTP headers, `bundle-inventory.json`, `runtime-checks.txt`, `qt-smoke.json`, and `python-tests.log` record the other checks.

## Fixture provenance and licensing

Author's public demo: [unacro/rpgmaker-mz-demo](https://github.com/unacro/rpgmaker-mz-demo), [playable web page](https://unacro.github.io/rpgmaker-mz-demo/).

- Commit: `81abd876e3c339fbd7d74323ad8817ad7011cade`.
- Archive: `https://github.com/unacro/rpgmaker-mz-demo/archive/81abd876e3c339fbd7d74323ad8817ad7011cade.zip`.
- Observed archive: **97,280,852 bytes**, SHA-256 `ed4ee4c57d1c7b4ababf8900c8e4eb2fc4df1a3094911a6cefa66209810d2b7c` (locally measured, not an author-published checksum).
- Expanded payload: **103,935,935 bytes**, 1,332 files; all original files remained byte-identical to the ZIP after both runs. Only test saves were added; first-run saves were preserved as `save-first-pass/` before the second run.
- `package.json`: local `index.html`, title `Project1`, 816×624 window; no background/native startup hook. Active plugins are **TextPicture** (Yoji Ojima) and **Custom** (unacro), not two original/custom plugins. Additional save-plugin files are present but inactive.
- Reviewed entry point, active plugins, native/network/deletion search results, event script commands and storage code. No child-process/shell execution was found; no script/plugin event commands were found in game data. MZ's filesystem write/rename/unlink functions implement saves/backups under the game copy's `save/`; XHR/fetch load game assets. This targeted review is not a formal security audit of bundled/minified libraries.
- Repository `LICENSE` is GPL-3.0. README credits Fusion Pixel Font and other work. RPG Maker stock scripts/assets and fonts may have separate terms; repository GPL must **not** be treated as blanket permission to redistribute them. This was personal testing of the author's publicly playable demo. No assets, runtime binaries or screenshots are added to tracked source. The restricted `gzqyl/rpgemu` fallback was not used.

## Runtime acquisition and checks

Actual command (absolute paths expanded in `install.json`):

```sh
R="$PWD/build/validation/mz-20260717"
go build -o "$R/nwjs-helper" ./cmd/nwjs-helper
"$R/nwjs-helper" install --root "$R/runtime" --version v0.94.1 --arch arm64
```

The helper itself downloaded, verified and extracted:

- `https://dl.nwjs.io/v0.94.1/nwjs-sdk-v0.94.1-osx-arm64.zip`
- **157,840,235 bytes**, SHA-256 **`aa66963cde27e5a8e776173d18893b23986fc17f2c5a4237a358f690fc7691d2`**.
- Independently fetched official `https://dl.nwjs.io/v0.94.1/SHASUMS256.txt`; exact archive entry matched. Manifest redirects/headers are retained. Installer JSONL records download/verify/extract completion and final `complete` event; exit 0. The helper deletes its staging ZIP on success, so a second archive copy/hash calculation was not retained or claimed.
- Extract progress ended at **386,634,498 bytes**. No manual runtime unzip was used.
- Main executable, framework, dylibs and helpers: **15 Mach-O files checked as arm64**, executable mode `0700`; **5 framework symlinks** resolved within the bundle with no dangling links.

**Signature caveat:** `codesign --verify --deep --strict --verbose=2 "$APP"` exited **1**:

```text
code has no resources but signature indicates they must be present
```

`codesign --display --verbose=4` reports an upstream **ad-hoc/linker signature**, no TeamIdentifier and no sealed resources. The unmodified executable nevertheless launched normally on this host. No re-signing, quarantine removal, Gatekeeper/SIP change, or security override was performed. This does **not** establish notarization or a working quarantined/Finder distribution experience. Preserve any OS refusal on another host as a blocker; do not bypass it.

## Real game behavior

Final screenshots/state, relative to the renderer evidence directory:

| Evidence | Actual check |
|---|---|
| `01-title.png` | Rendered title artwork, readable Chinese New Game/Continue/Options menu; New Game selected. |
| `02-new-game.png` | Enter key started map **1**, player **(8,7)**. |
| `03-movement.png` | Left/Up keys moved player to **(7,6)**, facing up (8). |
| `04-switch-event.png` | Enter activated nearby event; switch **2 changed false → true**. |
| `05-save-menu.png`, `06-saved.png` | Escape opened menu, Down selected Save, Enter opened save UI and wrote **file 1**. |
| `07-relaunch-title.png` | First process quit; a new process rendered title with Continue selected. |
| `08-load-menu.png`, `09-loaded.png` | Enter selected Continue and file 1; restored map **1**, **(7,6)**, direction **8**, switch **2=true**. |

All gameplay actions used **CDP `Input.dispatchKeyEvent`** through real engine UI. `Runtime.evaluate` only inspected state/versions (plus `nw.App.quit()` for cleanup); **no direct engine new-game/save/load APIs** were used. This is automated keyboard/menu coverage, not a human physical-keyboard test. Screenshots were inspected, not inferred from Popen success.

The first pass passed state/save/load assertions but its first title screenshot caught the opening animation before menu text appeared. The harness was corrected to wait for the command window to open; the complete second pass produced the readable screenshot above. Both passes' evidence remains available.

- Renderer type: PIXI WebGL; no claim about GPU acceleration/performance.
- WebAudio context reported `running`; **audible output was not verified**.
- No captured JS exceptions, failed resource requests, or engine error overlay after CDP attachment. Native stderr is **not clean**: missing optional bundled `Resources/app.nw`, macOS crash-info-reader/memory warnings and `SetApplicationIsDaemon ... -50` appear. They did not prevent these checks; their broader consequences were not investigated.
- CDP bound only to **127.0.0.1**, confirmed with `lsof`. Four runtime PIDs and all four debug ports were confirmed gone/closed afterward; no runtime/profile-associated child processes remained.

## Python/Qt integration coverage

```sh
/tmp/rpgm-qt-venv/bin/python scripts/validate-nwjs-qt.py "$R"
QT_QPA_PLATFORM=offscreen /tmp/rpgm-qt-venv/bin/python -m unittest discover -s tests -v
```

`validate-nwjs-qt.py` copies the installed runtime (preserving framework symlinks) into a newly created `qt-home/Library/Application Support/RPGM-Launcher`. It uses the **real RPGMLauncher window, HelperProcess/QProcess and built Go helper**. Only modal version-choice/message-box interaction is automated.

Observed: install button disabled management while busy; official lookup returned **320 versions**; selecting v0.94.1 invoked real `install --root ... --version v0.94.1 --arch arm64`; validated no-op returned `complete` without download progress; selector/settings persisted v0.94.1; controls recovered; Optimize Space remained disabled. **31 timer ticks** demonstrated event-loop activity. Screenshot: `qt-offscreen.png`. All **39 Python/Qt+safety tests passed** (9.730 s). The smoke does not test a fresh GUI-driven download, native Qt appearance, bundled-app discovery, or launcher Start Game/export.

## Observed resource/time measurements

- Disk before downloads: **461 GiB available** (`df -h`). Final retained validation tree: **931,044 KiB** (`du -sk`, includes runtime copy for Qt, fixture ZIP/assets, venv, saves and evidence).
- Helper network download + verification + extraction: **13.971 s**; fixture download in parallel: **15.737 s**; independent manifest fetch: **1.497 s**.
- Main runtime+fixture archive payload downloads: **255,121,087 bytes** total, plus small metadata and local-venv websocket-client package. No repeated runtime/archive download.
- Renderer passes: **21.168 s**, then **18.515 s** (harness measurement through successful restored-state assertion, not a performance benchmark).
- Qt real-helper no-op smoke: **0.850 s**, excluding copying/setup.
- Peak RSS/CPU, throughput benchmarks and human audio latency were **not measured**.

## Reproduce safely

**Current ARM-only CLI (not the historical invocation above):** use native darwin/arm64 Python and the rebuilt arm64 helper. Both validation scripts reject unsupported hosts and check the runtime's main Mach-O executable before creating profiles/evidence or launching. The Qt script additionally checks the helper and the arm64-only versions protocol. Main-executable validation is not a dylib/signature/whole-game compatibility guarantee. Intel/Rosetta is unsupported, not an untested fallback.

Use a fresh ignored/temp directory, a logged-in macOS arm64 GUI session, available disk, and your own reviewed/legal fixture copy. Download budget here is well below 2 GiB. Do not point these scripts at a personal game/save/profile directory. A local HOME, cache, TMPDIR and `--user-data-dir` are **hygiene, not an OS sandbox**; NW.js still has the user's privileges. No credentials are passed in the game subprocess environment. Do not run arbitrary unreviewed games.

```sh
mkdir -p build/validation
R=$(mktemp -d "$PWD/build/validation/mz-real.XXXXXX")
mkdir -p "$R/evidence" "$R/game"
go build -o "$R/nwjs-helper" ./cmd/nwjs-helper
# Stop on any failure; do not substitute a manually unzipped runtime.
"$R/nwjs-helper" install --root "$R/runtime" --version v0.94.1 \
  >"$R/evidence/install.stdout" 2>"$R/evidence/install.stderr"
curl --fail --location https://dl.nwjs.io/v0.94.1/SHASUMS256.txt \
  -o "$R/evidence/SHASUMS256.txt"
curl --fail --location \
  https://github.com/unacro/rpgmaker-mz-demo/archive/81abd876e3c339fbd7d74323ad8817ad7011cade.zip \
  -o "$R/fixture.zip"
python3 - "$R" <<'PY'
import hashlib, pathlib, stat, sys, zipfile
r = pathlib.Path(sys.argv[1])
assert hashlib.sha256((r/'fixture.zip').read_bytes()).hexdigest() == 'ed4ee4c57d1c7b4ababf8900c8e4eb2fc4df1a3094911a6cefa66209810d2b7c'
with zipfile.ZipFile(r/'fixture.zip') as z:
    assert sum(i.file_size for i in z.infolist()) < 200*1024*1024
    for i in z.infolist():
        p = pathlib.PurePosixPath(i.filename)
        assert not p.is_absolute() and '..' not in p.parts
        assert not stat.S_ISLNK(i.external_attr >> 16)
    z.extractall(r/'game')
PY
APP="$R/runtime/v0.94.1/nwjs.app"
file "$APP/Contents/MacOS/nwjs"
lipo -archs "$APP/Contents/MacOS/nwjs"
codesign --verify --deep --strict --verbose=2 "$APP"
# Review notices/package.json/js/main.js/active plugins/data events/save paths;
# inspect signature failures without altering signatures/security settings.
python3 -m venv "$R/venv"
"$R/venv/bin/pip" install websocket-client==1.9.2
"$R/venv/bin/python" scripts/validate-mz-runtime.py "$R" --reviewed-fixture
# Use a local venv with requirements-dev.txt installed for this separate smoke:
/path/to/qt-venv/bin/python scripts/validate-nwjs-qt.py "$R"
```

The real harness uses assertions as its live self-check. Never run it with Python `-O`. Use a fresh fixture/save directory for repetitions; existing file-1 saves are refused rather than deleted. The Qt smoke refuses to reuse `qt-home`. Artifacts remain for inspection; neither script deletes user data. If GUI launch/permissions fail, retain logs and perform the same New Game → movement/event → Save → quit/relaunch → Continue sequence manually only after resolving the blocker without weakening OS security.

**Uncovered:** other MZ maps/battles/plugins, MV, XP/VX/VX Ace, EasyRPG, Intel/Rosetta, other NW.js releases/macOS versions, long sessions, input devices, audibility, export/patching/cheats, and signed/notarized distribution. None should be inferred from this one-fixture success.
