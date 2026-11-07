# Python d56d1df → Go 機能照合（暫定）

> この文書は作成時点の比較記録であり、現在の互換性やテスト結果を示すものではありません。参照される `build/` 内の実験成果物は整理時に削除済みです。

## 範囲と数え方

基準は `d56d1df` の `src/python/main.py`、`nwjs_installer.py`、`runtime_arch.py` と、同版に既に存在する Go NW.js helper。現行は作業ツリー（未コミット変更を含む）の `src/go/internal/launcher/`、`src/go/internal/ui/`、`src/webui/`。旧 UI の関数単位ではなく、利用者の目的別に下表の **28 単位**を定義した。各行は等重みであり、行の分け方を変えると値も変わる。

- 静的な実装所在を確認できた「対応」26/28 = **92.9%**。残り「差異」2/28 は一部実装があるが、旧目的を全条件で満たすとは判断しない。これは動作の完全性・実ゲーム互換率・単体テストのコードカバレッジではない。
- いずれも実装の存在と主要経路の照合であり、全分岐や全入力の同値性を証明していない。特に「対応」は実ゲームで確認済みを意味しない。下表のテスト名は関連する自動テストの所在であり、全シナリオの合格証明ではない。
- 実ゲームの現行機能カバー率は **算定不能**。過去の MZ 1 fixture の直接 NW.js 起動と保存・再開は `nwjs-go-migration.md` に記録されているが、launcher の選択→パッチ→起動・書き出しの一括検証ではない。今回それを再実行していない。XP/VX/VX Ace、2000/2003、standalone の実ゲーム結果もない。
- 旧 default の全キー・値・enabled は下記のとおり比較済み。旧版の全分岐・保存済み設定の全組合せ・アーカイブ形式との逐語的な照合は未完了。数値を移行完了の宣言として扱わない。

| # | 旧機能の目的 | 現行の所在・根拠 | 状態 / 未検証・差異 |
|---|---|---|---|
| 1 | Apple Silicon 実行環境検査 | `runtime_arch.go`、`arm_only_test.go` | 対応 / macOS 各版は未実機検証 |
| 2 | NW.js 公開 arm64 版一覧 | `api.go`、`cli.go`、`app.js`、`api_test.go` | 対応 / 実 API の今回の応答未検証 |
| 3 | NW.js download・checksum・展開 | `install.go`、`archive.go`、`main_test.go` | 対応 / 今回実 download なし |
| 4 | NW.js 導入済み検出・選択 | `runtime_arch.go`、`controller.go`、`install_guard_test.go` | 対応 / 旧 receipt なし arm64 と不正 receipt の扱いは安全検査あり |
| 5 | 管理操作の排他とキャンセル | `lock_darwin.go`、`controller.go`、`main_test.go` | 対応 / 強制終了後の残骸は手動調査 |
| 6 | MKXP-Z の取得と更新 | `components.go`、`controller.go`、`components_test.go` | 対応 / API 障害時の固定 URL fallback を含む未コミット変更あり。実 download 未検証 |
| 7 | EasyRPG の取得と更新 | `components.go`、`components_test.go` | 差異 / metadata の取得失敗時は固定の lastSuccessfulBuild artifact にフォールバックし、版を未記録とする。実インストールと障害時の実用性は未検証 |
| 8 | RTP の取得・旧導入検出 | `components.go`、`legacy_resources_test.go` | 対応 / 旧資源検出の未コミット変更あり。実配布物は未検証 |
| 9 | SoundFont の取得・検証 | `components.go`、`components_test.go` | 対応 / 実音声未検証 |
| 10 | Kawariki の取得・旧導入検出 | `components.go`、`legacy_resources_test.go` | 対応 / 実ゲーム未検証 |
| 11 | 個別・全コンポーネント削除 | `components.go`、`app.js`、`components_test.go` | 対応 / 未知データやゲームは削除対象外 |
| 12 | 基本設定の読込・保存 | `settings.go`、`controller.go`、`settings_test.go` | 対応 / 破損時は保全してエラー |
| 13 | MKXP-Z 詳細設定の編集・reset | `defaults.go`、`settings.go`、`ui/form.go`、`settings_test.go` | 対応 / default 値・enabled 各 61 キー一致。実 runtime の全オプションは未検証 |
| 14 | EasyRPG 詳細設定の編集・reset | `defaults.go`、`ui/form.go`、`gate_fixes_test.go` | 対応 / default 値 56・enabled 55 キー一致。実 runtime の全オプションは未検証 |
| 15 | フォルダ選択とエンジン判定 | `game.go`、`controller.go`、`game_test.go` | 対応 / 実 native folder dialog の操作未検証 |
| 16 | RTP・RGSS 版推定 | `game.go:GetRTPValue`、`game_test.go` | 対応 / 個別作品未検証 |
| 17 | package.json の補修 | `game.go:PreparePackage`、`game_test.go` | 差異 / 旧 chardet と異なり UTF-8 優先、次に可逆 CP932 と解釈。別の文字コードでも CP932 として受理され得るため原本の意味の保全は保証できない |
| 18 | NW.js 保護 JS の配置・無効化 | `game.go`、`game_test.go` | 対応 / 独自スクリプト衝突時は上書きせず警告 |
| 19 | getPixel パッチ適用・除去 | `game.go:PatchGetPixel`、`game_test.go` | 対応 / 保守的 JS scanner は全 JS 構文の parser ではない |
| 20 | NW.js ゲーム起動 | `launch.go`、`launch_test.go` | 対応 / 実ゲームの launcher 経由未検証 |
| 21 | MKXP-Z 起動と RTP/SoundFont/Kawariki 設定 | `engines.go`、`launch.go`、`launch_test.go` | 対応 / 実ゲーム未検証 |
| 22 | EasyRPG 起動と追加引数 | `engines.go`、`launch.go`、`launch_test.go` | 対応 / 実ゲーム未検証 |
| 23 | 必要な資源の導入案内 | `app.js:readyGame`、`MissingComponents`、`install_guard_test.go` | 対応 / 実 UI 対話未検証 |
| 24 | NW.js standalone 書き出し | `export.go`、`export_test.go` | 対応 / 実 standalone 起動未検証 |
| 25 | MKXP-Z standalone 書き出し | `export.go`、`export_test.go` | 対応 / 実 standalone 起動未検証 |
| 26 | EasyRPG standalone 書き出し・追加引数 | `export.go`、`export_test.go` | 対応 / 実 standalone 起動未検証 |
| 27 | ログの表示 | `controller.go:Logs`、`log_compat_test.go`、`app.js` | 対応 / 旧 `log.txt` を保全して末尾を表示。新しい操作ログは `launcher.log`、ゲーム出力は `game.log` に追記し、各末尾を表示 |
| 28 | 操作説明・クレジットの表示 | `README.md`、`src/webui/index.html` | 対応 / 旧ダイアログの情報は画面内の「使い方」「謝辞」に収録。画面構成は異なる |

## 分母から除外した意図的撤去・安全変更

- `a6c6e96` で旧基準より前に撤去済みのチート注入・除去、`game_en.exe` 展開、外部セーブ編集サイトは復活させない。通常セーブ・ロードは別機能。
- 旧基準も ARM 専用（`3e78048`）。Intel/Rosetta および旧 `--arch` / `--rosetta` は対象外。`Optimize Space` は旧基準ですでに無効。
- 設定バージョン変更に伴う一括削除、選択時のゲーム書き換え、広範な権限変更や既存資源の無検証上書きは安全な現設計に戻さない。安全上の拒否は一律に旧互換欠落として数えない。
- Qt/Python subprocess・helper JSONL の GUI 内部通信は目的ではなく実装手段。現 GUI は Go backend を直接呼ぶ。NW.js helper 自体は開発 CLI として残る。

## default 設定の機械的比較

旧 `d56d1df:src/python/main.py` の4つの辞書を Python AST の `literal_eval` で抽出し、現作業ツリーの `DefaultAdvancedSettings()` を一時 Go テストで実行して JSON 出力した結果と比較した。キー集合、共通キーの値および JSON 型を比較し、数値の表記も Python の int / float として区別した。比較に使った一時テストは削除済み。

| 辞書 | 旧キー数 | 現キー数 | 共通 | 旧のみ | 現のみ | 共通キーの値・型の差 |
|---|---:|---:|---:|---:|---:|---:|
| MKXP-Z 値 | 61 | 61 | 61 | 0 | 0 | 0 |
| MKXP-Z enabled | 61 | 61 | 61 | 0 | 0 | 0 |
| EasyRPG 値 | 56 | 56 | 56 | 0 | 0 | 0 |
| EasyRPG enabled | 55 | 55 | 55 | 0 | 0 | 0 |
| 合計（辞書ごとの延べ数） | 233 | 233 | 233 | 0 | 0 | 0 |

MKXP-Z の enabled は `rgssVersion`、`gameFolder`、`preloadScript`、`RTP`、`midiSoundFont` が `"Force"`、残り 56 は boolean `false`。EasyRPG の enabled 55 はすべて boolean `false`。`--project-path` は両版とも値辞書には存在し enabled 辞書には存在しない。これは旧版由来であり、片方だけの欠落ではない。

基本設定も旧 `save_settings` の辞書キーを AST で抽出し、現 `DefaultSettings("/HOME")` の JSON 出力と比較した結果、双方 8 キー、旧のみ・現のみとも 0。値は旧が UI・保存時状態から生成するため同一時点の固定辞書ではない。旧初期状態と読み込み時 fallback、および現 default を照合すると、`optimize_space=false`、`disable_protection=false`、`patch_getpixel=true`、`last_selected_folder=~/Downloads`、`last_commit_sha=""`、`last_build_number=0` は一致する。`last_selected_version` は旧 UI の選択状態に依存し、現 default は空文字。`launcher_version` は旧の保存時には `current_version`、読み込み fallback は空文字、現 default は空文字で、保存値と初期値は単純比較できない。旧 UI の実際の初期選択値は今回実行確認していない。

この比較で default の意図しない欠落は検出されなかったため、実装変更はしていない。保存済みファイルの移行・フォーム操作・起動時の全組合せや、各 runtime が値をどう解釈するかをこの比較だけで保証しない。

## EasyRPG 配布物の追加照合

旧 `d56d1df` は固定の `/downloads/macos/EasyRPG-Player-macos.app.zip` を取得し、別途 job API の先頭 build 番号を保存した。両者が同一 build である保証はない。現行は成功済み build の metadata に記載された ZIP をその build 番号の URL から取得し、展開後の構造・arm64 を検証してから置き換える。metadata の取得失敗時は固定の lastSuccessfulBuild artifact URL にフォールバックし、以前の build 番号を消して版不明として扱う。メタデータ自体が不正な場合は拒否する。ZIP と実行ファイルの検証は維持する。

今回の実 CI 応答は成功 build 1969 と `osxbuild/EasyRPG-Player-macos.app.zip` を返し、番号付き artifact URL と旧 URL の HEAD はともに HTTP 200、同一 Content-Length と Last-Modified を返した。ヘッダーだけでは内容の同一性や将来の可用性は保証されず、ZIP の実取得・インストールは行っていない。

## 今回の実行と残課題

`mise run test` は通常 Go test、race test、vet が成功し、`node src/webui/app.test.cjs` は 34 件成功した（いずれも未コミット変更を含む作業ツリー）。実ゲーム、実ネットワーク配布物、native WebKit smoke、物理入力、音声、公証・Gatekeeper は今回実行していない。

後続の #27・#28 照合では、旧版は `log.txt` を起動時に消去し、操作・ゲーム出力を同じファイルに記録していたことを確認した。現行は旧ファイルを消さず、別ファイルに追記して画面に各末尾を表示する。全履歴の画面表示や自動ローテーションは行わず、保存には空き容量とログ内のパス等の機密情報に注意が必要。旧 Instructions の動画、各エンジンの手順、オプション、書き出し・削除と Credits の全氏名は現画面にある。旧動画は旧画面向け。

差異 #7、#17 は目的への影響と安全性を更に検証する必要がある。特に文字コード対応を推測で増やしてゲームを書き換えないこと。削除メニューと GUI 全経路の詳細照合、実 fixture を使う launcher 経由の起動・各エンジンの standalone 検証も未完了。

### 文字コード差異の追加評価

旧 `prepare_package_json` は chardet の推定結果で読み書きし、旧 MKXP-Z の `mkxp.json` も同様に推定していた。現行の `dataContractDecode` は UTF-8 を優先し、それ以外は CP932 に変換して元バイトへ往復できる場合に受理する。往復一致は文字コードを特定する証拠ではない。別の文字コードが CP932 として受理されれば、`package.json` の変更時や MKXP-Z の設定再生成時に文字の意味が変わる可能性がある。変換不能な入力は書き込み前に拒否するが、すべての非対応文字コードを拒否できるわけではない。`Game.ini` にも同じデコードを使うが、これは読み取りのみ。

旧 chardet の対応範囲だけでは実際に使用された他の文字コードや正しい解釈は特定できず、今回その実ゲーム入力を確認していない。合成入力で旧 chardet 5.2.0 と比較したところ、旧版も文字コードの誤判定によって値を変更した。UTF-8 BOM 付き mkxp.json を現行でも読み取れるよう修正した。新たな文字コードの自動推測や変換は追加しない。既存テストには CP932 の package 補修と Game.ini の読み取り・不正入力の保全があるが、別文字コードとの曖昧性は解決していない。
