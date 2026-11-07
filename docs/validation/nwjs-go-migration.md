# Incremental Go migration: real MZ validation

> 保管状況：以下は過去の検証記録です。記載された `build/`・`dist/` 内の実験ゲーム、ランタイム、ログ、画像は整理時に削除済みで、現存する証跡へのリンクではありません。検証用ソースは維持していますが、再実行にはレビュー済みの入力を別途準備する必要があります。

> 現在はGo backend + Wails/system WKWebView + plain webuiです。ソースは`src/go/`・`src/webui/`・`src/javascript/`、配布素材は`packaging/macos/arm64/`、build/smokeは`scripts/shell/`、MZ harnessは`src/go/cmd/validate-mz/`です。Python/Qtのソース・tests・scripts・specとproject venvは撤去済みで、現行miseはGo 1.27.3のみです。以下のPython結果・旧path・旧CLIは当時の履歴として維持し、末尾「Reproduce safely」を現在のGo手順に分けています。配置整理の歴史証跡は`build/validation/layout/`、Go移行gateは`build/validation/python-zero/final-parity-gate/`です。

> ARM専用化の現状注記：ランチャー／Go helperはmacOS Apple Silicon arm64のみ対応します。NW.js一覧もarm64配布のある版のみで、旧`--arch`・`--rosetta`は拒否します。本文の320版・旧CLI・Rosetta同意・3アーキテクチャなどは過去の事実として残し、現在の再実行手順は末尾「Reproduce safely」に分けています。ARM専用化でこの実ゲーム試験を再実行したわけではありません。

> 現状注記：この記録は機能撤去前の検証です。現在はチート用JS/CSSの注入・除去、`game_en.exe`からの英語版展開、外部セーブ編集サイトへの導線を撤去しています。以下のテスト件数・条件・結果は当時の履歴であり、撤去後の実ゲーム再試験を示すものではありません。通常のゲーム内セーブ・ロード試験の記録はそのまま残しています。

> 回帰テストの現状：撤去済み機能・旧配置・依存ツールの不在だけを示す番兵検査は含めません。以下の不在検査・コマンド拒否wrapperによる結果は当時の履歴として保持し、現役の正常動作・原本保護・危険入力拒否・cleanup検査は継続します。

## Go-only tooling acceptance (2026-11-05; implementation check, pending independent review)

- Project-only `.venv` and the 13 legacy Python source/test/script/spec files were removed after the independent Go parity gate. The global mise Python executable was hash-checked and left untouched; historical `build/validation/` artifacts were retained.
- With Python/pip/PyInstaller and development Node/npm/npx commands denied by PATH wrappers, and PYTHONHOME/PYTHONPATH/VIRTUAL_ENV unset: mise setup, two successive canonical builds, uncached default/production/smoke/combined Go test/race/vet and mod verify passed. Wrapper resolution was checked inside mise; denied invocations: zero. This does **not** claim that Python is absent from the OS. Static subprocess checks complement the wrappers; the MZ child intentionally uses a minimal system PATH and NW.js contains its own Node runtime.
- Canonical `dist/RPG-Maker-Launcher.app` is a fresh Go bundle, arm64, asset-byte/plist/ad-hoc verified, with no Python/Qt framework leftovers. Distribution tests cover arbitrary output refusal, symlinks, failed build/publish/restore, preserved backup, and a destination appearing during publish (no nested app merge).
- Fresh native WebKit: 31 checks, heartbeat +4 while busy, cleanup-held cancellation; production startup isolated from smoke. Injected JS exception and suppressed-ready cases exited **1** themselves, without an external timeout/kill. WebKit visibility was hidden; physical UI/accessibility are not proven. The first smoke copy changed only directory/symlink modes under umask 077; retained failure evidence led to a RED→GREEN mode-preserving copy regression. The corrected source/copy trees match in bytes/modes/links.
- Fresh Go MZ run: 9 PNG/state pairs inspected, keyboard Save → process restart → Continue/Load restored map 1, (7,6), direction 8, switch 2=true. All 1,332 fixture files and original ZIP/runtime hashes unchanged; both owned groups and debug ports cleaned up. Upstream runtime strict-signature failure remains recorded, without re-signing or OS security changes.
- Evidence: `build/validation/python-zero/final-tooling/` (ignored/local). No new game/runtime download. Developer ID/notarization, Finder/Gatekeeper, VoiceOver/IME/native-dialog/physical input, macOS13.3 and audibility remain unverified.

## Layout acceptance (2026-11-05; historical Python independent review)

- macOS 26.5.1 arm64 / Python 3.11.18 / Go 1.27.3: `mise tasks validate`, `mise run setup`, `pip check`, `mise run test` (**62 Python tests, no skips**), uncached Go `test` / `test -race` / `vet` passed. The 30 moves preserve all Go/JS/icon/dependency bytes; remaining code edits only resolve relocated paths. Other-CWD shell build/source imports, local documentation links, source/frozen resource tests and spec inputs passed.
- From the repository root, `mise exec -- python -m PyInstaller --clean --noconfirm packaging/macos/arm64/arm64.spec` rebuilt canonical `dist/RPG-Maker-Launcher.app`. Bundled JS/Standalone/icon bytes match source; main/helper are arm64; local ad-hoc strict codesign passed. Source and frozen GUI each stayed alive offscreen for 3 seconds in separate fresh HOMEs without traceback; bundled helper no-op passed.
- Reused the previously reviewed author fixture ZIP and helper-installed NW.js 0.94.1 in a **fresh copy**, with no game/runtime download. Current CLI no-op passed; relocated harness completed title → keyboard New Game → movement/event → menu Save → quit/relaunch → Continue/Load. All 9 screenshots and state JSON were inspected: map 1, (7,6), direction 8, switch 2=true restored. Original 1,332 fixture files and previous game/save/profile/runtime/HOME stayed unchanged; both runtime processes/debug ports closed. Real Qt/QProcess no-op also passed (65 arm64 releases, 40 timer ticks).
- Local-only evidence: `build/validation/layout/independent-review/`; game proof: `mz-final/evidence/renderer-1793861038191683000/`. NW.js strict codesign still exits 1 (`code has no resources but signature indicates they must be present`); native stderr retains the known app.nw/macOS warnings. No runtime re-signing or OS protection changes. Audibility, other games, launcher Start Game/patch/export, Developer ID/notarization and Finder/Gatekeeper remain untested.

## ARM-only regression checks (historical Python; separate from the game run)

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

**Current ARM-only Go CLI (not the historical Python invocation above):** use native darwin/arm64 Go 1.27.3, Xcode Command Line Tools and a logged-in GUI session. No Python, Qt, venv or development Node CLI is required. The Go harness checks the host, physical absolute input paths, pinned fixture ZIP hash, safe archive members, and arm64 runtime before launching. Main-executable validation is not a dylib/signature/whole-game compatibility guarantee. Intel/Rosetta is unsupported.

Use existing reviewed/legal inputs and a **nonexistent** output root whose physical parent already exists. Do not point it at personal game/save/profile data. HOME/cache/TMP/profile separation is **hygiene, not an OS sandbox**; NW.js still has user privileges. Review the fixture's notices, entry point, active plugins, data events and save behavior before explicitly consenting. Do not weaken OS security or re-sign the external runtime.

From the repository root (the example reuses local evidence inputs, which are not shipped in Git):

```sh
mise run setup
mise run test
mkdir -p build/validation
R=$(pwd -P)
# Both inputs must already exist; missing inputs are errors, not downloads/skips.
# mz-go-fresh must NOT exist. Choose a new name for each run.
mise run validate -- \
  --root "$R/build/validation/mz-go-fresh" \
  --fixture-zip "$R/build/validation/mz-20260717/fixture.zip" \
  --runtime "$R/build/validation/mz-20260717/runtime/v0.94.1" \
  --reviewed-fixture

# Separate opt-in native WebKit GUI acceptance; gui-go-fresh must NOT exist.
mise run smoke -- "$R/build/validation/gui-go-fresh" \
  "$R/build/validation/mz-20260717/runtime/v0.94.1" --reviewed-runtime
```

Equivalent MZ entry point: `mise exec -- go -C src/go run ./cmd/validate-mz` with the same four arguments. It copies the pinned archive/runtime, never downloads or patches them, drives only keyboard input through CDP for gameplay, checks Save → restart → Load, records nine PNG/state pairs, rechecks all 1,332 source files and source hashes, and cleans up only owned process groups/debug ports. Inspect `evidence/result.json`, cleanup JSON, images and state JSON; process startup alone is not success. The known upstream strict-signature failure is recorded, not repaired. Unexpected signature/OS failures are blockers.

The GUI smoke builds a dedicated smoke-tagged app, creates a fresh sibling HOME/root with an ownership marker, copies the reviewed runtime and runs 31 WebKit checks. Production builds do not expose the smoke service. Artifacts remain for inspection; repeated roots are refused instead of deleting saves. Normal `mise run test` never starts these native acceptance runs. If launch/permissions fail, retain logs and resolve the blocker without security bypasses before any manual gameplay.

**Uncovered:** other MZ maps/battles/plugins, MV, XP/VX/VX Ace, EasyRPG, Intel/Rosetta, other NW.js releases/macOS versions, long sessions, input devices, audibility, export/patching/cheats, and signed/notarized distribution. None should be inferred from this one-fixture success.
