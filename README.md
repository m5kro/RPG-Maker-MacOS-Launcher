<img src="assets/icons/icon.png" height="200" alt="RPG Maker Launcher icon">

# RPG-Maker-MacOS-Launcher

RPG Maker製のゲームを**macOSのApple Silicon（arm64）専用**で動かすために、実行環境の導入とゲームの起動をまとめて扱うランチャーです。ゲーム制作ツールそのものではなく、手元にあるゲームを、その形式に合ったmacOS向けランタイムで起動します。MV・MZにはNW.js、XP・VX・VX AceにはMKXP-Z、2000・2003にはEasyRPGを使う構成を元プロジェクトから引き継いでいます。すべてのゲームやプラグインで動作することを保証するものではありません。

このforkは、[m5kro/RPG-Maker-MacOS-Launcher](https://github.com/m5kro/RPG-Maker-MacOS-Launcher)をもとに、安全性の見直しと段階的なGo移行を進めている派生版です。上流の「開発完了、必要に応じてバグ修正のみ」という案内は元プロジェクトについてのもので、このforkの作業完了を意味しません。上流では、より汎用的な後継として[Xenolauncher](https://github.com/m5kro/Xenolauncher)も案内されています。

今回Goに移したのは、**NW.jsのバージョン一覧取得と、検証付きインストールだけ**です。画面は引き続きPython/PySide6で動き、ゲームの起動・書き出し、互換性パッチ、MKXP-Z/EasyRPGの取得もPython側に残っています。全体をGoで書き直したわけではなく、Goを使うことでNW.jsなどのゲーム用ランタイムが不要になるわけでもありません。

> [!WARNING]
> 現時点では配布用の署名・公証を保証していません。実機検証に使ったNW.jsは、リソースの署名封印がないため厳密な`codesign`検証に失敗しました。このホストではセキュリティ回避操作なしで起動しましたが、別のMacやFinder/Gatekeeper経由で起動できることの証明にはなりません。OSが起動を拒否した場合は、保護機能を無効にして続行しないでください。

## なぜ、まずNW.jsの取得処理をGoに移したのか

ゲームを起動する前のダウンロードには、通信待ち、検証、展開、キャンセル、既存版との共存が伴います。今回はこのまとまりを小さな独立プログラムに切り出し、画面を待たせずに処理しながら、失敗時の扱いを明確にすることを優先しました。GUIまで同時に作り直すのではなく、既存の画面を残して境界のはっきりした部分から移行しています。ゲームの速度向上や、互換性の拡大を実証した変更ではありません。

`Install NWJS Version`を押すと、Qtの`QProcess`がGo helperの`versions`を非同期で起動し、選択後に`install`を実行します。ダウンロード・検証・展開の進捗はJSONLで画面に伝えます。正常終了と期待する完了通知の両方を確認してから、その版を選択して設定を保存します。helperが見つからない場合に、旧Python版の同期ダウンロードへ戻る処理はありません。

**Intel Mac・Rosetta実行は非対応です。** Apple SiliconでもIntel版PythonをRosettaで実行している場合は、ログの消去や設定の変更より前に起動を拒否し、native arm64 Pythonを案内します。NW.jsの一覧には公式のmacOS arm64配布がある版だけを出し、x64へのフォールバックやRosetta同意・起動分岐はありません。helperの導入CLIは`install --root ABS --version vX.Y.Z`のみで、旧`--arch`・`--rosetta`はエラーです。

MKXP-Zの既存`Z-universal.zip`とEasyRPGの既存公式CI URLは維持しますが、配布名やサイト表記だけで対応を判断しません。取得後・検出時・起動／書き出し前に、主実行ファイルのMach-O thin/FATヘッダーを標準ライブラリで検査します。arm64 sliceのある外部Universalバイナリは受け入れ、起動は`/usr/bin/arch -arm64`に固定します（EasyRPG書き出し用スクリプトも同様）。x86 sliceの削除や再署名は行いません。Intel-only・破損・欠落した実行ファイルは明示的に拒否し、別配布へ勝手に切り替えません。これは全dylib・ゲーム互換性・署名の保証ではありません。ランチャー実行時に`lipo`やCommand Line Toolsは不要です。

EasyRPG公式CIの[build 1969ログ](https://ci.easyrpg.org/job/player-macos/1969/consoleText)には`CMAKE_OSX_ARCHITECTURES=x86_64;arm64`とUniversal署名・ZIP転送の記録があります。ただし今後取得する内容も必ず検査します。EasyRPG/MKXP-Z更新は、新しいダウンロード・展開・arm64確認が終わるまで旧版を消さず、stagingから切り替えます。切替失敗時は旧版を復元し、復元自体が失敗した場合も管理フォルダーの`.アプリ名.previous`を回復用に残します。

### ダウンロードと既存版の扱い

helperは公式の`SHASUMS256.txt`とダウンロードしたアーカイブのSHA-256を照合してから展開します。チェックサムの欠落、不正なアーカイブ、通信エラー、アーキテクチャの不一致があれば、そのまま導入を進めずエラーにします。作業場所は`~/Library/Application Support/RPGM-Launcher`内の専用の非公開一時ディレクトリ（`.nwjs-stage-*`）です。導入完了までは作業用ディレクトリを版一覧に出さず、一覧にはarm64で実行可能な`nwjs.app/Contents/MacOS/nwjs`がある版だけを表示します。旧x64ランタイム、x64/Rosetta receipt、`run-with-rosetta`マーカーのある版は隠し、直接起動・書き出しを要求されてもゲーム補修前に拒否します。旧データは自動削除しません。

失敗したからといって、既存版を削除したり置き換えたりはしません。helperで導入済みの同じ版は再検証し、条件が一致すれば再ダウンロードせずに完了します。一方、旧ランチャーが導入した版、不完全な版、条件の合わない同名ディレクトリは、勝手に上書きせず残します。置き換えたい場合は先にバックアップし、正常な旧版なら明示的にアンインストールしてください。一覧に出ない不完全なディレクトリは、内容の手動確認が必要になることがあります。

キャンセル時はまずSIGTERMで後片付けを促し、3秒以内に終了しなければSIGKILLに切り替えます。ウィンドウを閉じる操作でも、画面のイベント処理を止めずにhelperの終了を待ちます。一覧取得・導入中は、起動・書き出し・アンインストールなどの管理操作を無効にします。ただし強制終了や電源断では、一時ディレクトリや予約済みの空の版ディレクトリが残る場合があります。また、導入先への配置後にキャンセルすると、有効な版が残っても自動選択されないことがあります。再起動すれば一覧に現れますが、キャンセルによって配置済みの版が巻き戻されるわけではありません。

Go側の導入とPython側のNW.js削除・`Uninstall All`は、OSの`flock`で同じ`.nwjs-install.lock`を使い、競合する操作を待たずに拒否します。`Uninstall All`でも管理ルートとロックファイルの実体は残します。**ランチャーやhelperの動作中に、このロックを削除しないでください。** この取り決めを知らない旧ランチャーや外部ツールとは排他できないため、同時実行は避けてください。残った作業ディレクトリを調べるのも、すべてのインストーラーを停止してからにしてください。

## ゲーム原本を守るために変えたこと

日本の利用者向けに機能を整理し、`game_en.exe`からの英語版展開、チート用JS/CSSの注入・除去、外部セーブ改造サイトを開く`Open Save Editor`ボタンを撤去しました。通常のゲーム内セーブ・ロードは対象外です。EasyRPGの汎用的な言語・文字コード対応、保護用JS、`getPixel`パッチ、GoによるNW.js導入は残します。全UIの日本語化や海外ゲームの利用制限を行う変更ではありません。

過去に注入・変更された`main.js`、チート用ファイル、セーブデータは自動で元に戻しません。復元が必要な場合はバックアップや配布元の原本と確認してください。ゲーム原本に含まれる`game_en.exe`や英語素材も勝手に削除せず、書き出し時は通常どおりコピーします。

ゲームフォルダーを選んだだけでは、`package.json`を書き換えたり、ゲーム名に対応するユーザープロファイルを削除したりしないようにしました。起動時の共有NW.jsプロファイル削除、ゲームフォルダー全体への再帰的な`chown`/`chmod`、設定のバージョン違いを理由にした設定一括削除も撤去しています。ゲーム独自のバックグラウンドスクリプトは保持し、保護用スクリプトと衝突した場合も勝手に置き換えません。原本内のWindows向けファイルを削除していた`Optimize Space`は無効です。

これは、ランチャーのすべての操作が非破壊になったという意味ではありません。保護用JSの配置、`getPixel`パッチ、`package.json`などのJSON補修には、ゲーム原本を書き換える経路が残っています。**ゲームとセーブをバックアップし、試すときは原本ではなく専用のコピーを使ってください。** 信頼できないゲームを安全に隔離するOSサンドボックスでもありません。

## どこまで確認できているか

ARM専用化では、先に失敗するテストを記録したうえでPython/Qt **58件（skipなし）**が成功しました。native hostの副作用なし拒否、thin/FATの不正ヘッダー・Intel-only拒否、Universalのarm64許容、旧x64／マーカーの非破壊拒否、起動・書き出し前の検査、外部ランタイム更新失敗時の旧版保持と復元失敗時の回復コピー保持を含みます。arm64 helperとarm64 PyInstaller `.app`をビルドし、同梱主実行ファイル／helperのarm64検査、隔離HOMEでのoffscreen起動3秒、ローカルad-hoc署名の厳密検証も成功しました。RED/GREEN・ビルド記録は無視対象の`build/validation/arm-only/`です。実ゲームの再試験や外部ランタイムの新規ダウンロード、Developer ID署名・公証・Finder/Gatekeeper検証は行っていません。

以下のGo移行・mise切替時のテスト件数、320版の一覧、3アーキテクチャのビルド、Rosetta同意、実ゲーム試験は、機能撤去・ARM専用化より前の履歴です。現在の対応範囲やARM専用化後の実ゲーム再試験を示すものではありません。

Go移行時の検証では、Python/Qt連携23件と安全性16件の計39テスト、およびGoの`test`・`test -race`・`vet`が成功しました。Qt連携は実際のイベントループと偽の実行可能helperを使い、分割されたUTF-8/JSONL、末尾改行なし、一覧取得から導入への遷移、起動失敗、異常終了、不正・欠落した完了通知、Rosetta同意、キャンセルと強制終了、終了待ち、操作の無効化、成功時だけの更新を確認しています。ビルド済みの実Go helperとPythonの間で、同じロックが効くことも確認しました。

Go移行時の実機検証では、Apple Silicon上でGo helperからNW.js SDK 0.94.1 arm64を取得・検証・導入し、公開MZ 1.8.0デモ1本で、タイトルからNew Game、移動、イベント、メニューでの保存、プロセス終了と再起動、Continueからのロードまでを2回確認しました。操作はCDPによるキーボードイベントで、エンジンの保存・ロードAPIを直接呼んだ結果ではありません。原本1,332ファイルは不変でした。ゲームの出典・固定コミット、ハッシュ、画像や状態JSONの記録先、署名失敗の詳細と再現手順は、[検証記録と制約](docs/validation/nwjs-go-migration.md)にまとめています。

このゲーム試験はhelperが導入したランタイムを直接起動したものです。**ランチャーの`Start Game`、パッチ、書き出しまで通した実ゲーム検証ではありません。** 別途行った実Qt→Goの接続試験も、導入済みの同じ版を再検証して完了する経路であり、GUIから初回ダウンロードを最後まで通す試験ではありません。

音声が実際に聞こえること、ほかのMZ作品やプラグイン、MVやそれ以外のエンジン、長時間動作は未検証です。Intel/RosettaおよびUniversal2版ランチャーの配布は現在の対象外です。MKXP-Z/EasyRPGや書き出しなどの既存機能はコードに残っていますが、今回のGo移行で動作保証が追加されたわけではありません。配布用署名・公証とGatekeeper/Finder経由の挙動も未確認です。

## ソースから試す・ビルドする（macOS Apple Silicon arm64のみ）

このforkのソースを取得し、リポジトリのルートで以下を実行してください。[上流のビルド済みリリース](https://github.com/m5kro/RPG-Maker-MacOS-Launcher/releases/latest)は元プロジェクトの配布物であり、ここで説明しているGo移行を含むものではありません。

ビルドに必要なのはApple Silicon Mac、native arm64 Python、[mise](https://mise.jdx.dev/getting-started.html)とXcode Command Line Tools（ビルド成果物の検査に`lipo`を使用）です。`mise.toml`でPython **3.11.18**とGo **1.27.3**を完全な版番号で固定しています（`src/go/go.mod`の最低要件は1.25のままです）。`src/python/requirements-dev.txt`で固定したQt 6.12のwheelは**macOS 14以上**を必要とします。古いmacOSでは別のQt・依存関係の組み合わせを検証する必要があります。Goはhelperのビルドに使い、完成したアプリにはhelperの実行ファイルを同梱します。

以前の検証で使ったPython 3.11.2はmiseのmacOS arm64向け配布バイナリにないため、古い版をソースビルドする代わりに、`mise latest python@3.11`と配布バイナリ一覧で確認した3.11系の最新patch、3.11.18を採用しました。3.12以上には上げていません。miseの通常のTLS検証で取得したnative Pythonを使い、標準の`venv`でプロジェクト内の`.venv`を自動作成・選択します。手動のactivateやシステム全体へのpipインストールは不要です。別のPythonで作成済みの`.venv`は自動置換されないため、必要なら退避してから作り直してください。

```sh
# mise.tomlの内容を確認してから、このプロジェクトを信頼します。
mise trust
mise install
mise run setup
mise run test

# NW.js helperだけを生成します（GUIアプリのビルドではありません）。
# sh scripts/shell/build-nwjs-helper.shを実行し、CGO_ENABLED=0で
# arm64だけを生成・検査して、無視対象のbuild/に保存します。
# 過去のx64/Universal2成果物は自動削除しませんが、利用しません。
mise run build

# ソースからGUIを起動します（先にhelperをビルドします）。
mise run run
```

MV・MZを試す場合は、画面の`Install NWJS Version`からゲームに合う版を導入し、`Select Game Folder`でバックアップ済みのゲームコピーを選びます。通常はその後に`Start Game`で起動する流れですが、前述のとおり今回の実ゲーム検証はこのボタン経由ではありません。パッチなどの設定を確認し、原本ではなく検証用コピーで試してください。元の操作説明として、[xem](https://forums.rpgmakerweb.com/index.php?members/xem.159435/)/[pgm](https://www.youtube.com/channel/UCWTsqPj8SD8rHL9gsBZI59Q)による[解説動画](https://www.youtube.com/watch?v=NIRiKIjhXHk)もありますが、このforkの変更前の案内です。

ソース実行時は常に`build/helpers/arm64/nwjs-helper`を使います。探索の基準はプロジェクトの`__file__`からの絶対パスで、`PATH`ではありません。helperがない、または実行できない場合は、ビルドコマンドを含むエラーになります。

### ソースと配布ファイルの配置

```text
src/python/                  main.py・NW.js連携・ARM検査・requirements-dev.txt
src/go/                      go.mod・cmd/nwjs-helper/（Goテストも同じ場所）
src/javascript/              bg.js・disable-child.js・disable-net.js
tests/python/                Pythonテスト・Mach-O fixture
packaging/macos/arm64/        arm64.spec・EasyRPG-Standalone
assets/icons/                PNG・ICNSアイコン
scripts/python/              validate-mz-runtime.py・validate-nwjs-qt.py
scripts/shell/               build-nwjs-helper.sh
```

言語別のソースとmacOS arm64向け配布素材を分離しています。GUIの入口は`src/python/main.py`で、`mise run run`から実行します。`mise`は`PYTHONPATH`を`src/python/`の絶対パスに設定し、テストは`tests/python/`を探索、Goコマンドは`go -C src/go ...`で実行します。ソース実行時のJS・Standalone・helperは起動時の作業ディレクトリに依存せず、ビルド済みアプリ内では`sys._MEIPASS`直下の同梱ファイルを使います。root shimや`go.work`は置きません。Windowsでは原本ゲームのEXEを直接実行するため、追加ランタイムやWindows向け配布階層は作りません。

README・LICENSE・mise設定・`.gitignore`・`.github/`・`docs/`はルートのままです。無視対象の`build/`・`dist/`・`.venv/`と過去の検証証跡は移動・一括削除しません。

### テストとアプリ化

Python/Qtの通常テストはネットワークや実ゲームを使わず、一時HOMEと偽helperで実行できます。`mise run test`は`build`に依存し、Pythonと実Go helperのロック連携テスト1件が未ビルドでスキップされるのを防ぎます。Goの`test`・`test -race`・`vet`とoffscreenのPythonテストを順に実行し、毎回のpipインストールは行いません。依存関係の導入は最初に`mise run setup`で済ませてください。実ゲームによる追加検証は別途[検証記録の手順](docs/validation/nwjs-go-migration.md#reproduce-safely)に従い、専用の作業場所と利用条件を確認したゲームコピーで行います。

```sh
mise run test

# 配布はarm64のみ。native arm64 Python/Qtでビルドします。
mise run build
mise exec -- python -m PyInstaller --noconfirm packaging/macos/arm64/arm64.spec
```

`packaging/macos/arm64/arm64.spec`は`build/helpers/arm64/nwjs-helper`を、データではなくバイナリとして同梱します。x64/Universal2用specは撤去しました。アプリ内では、同梱されたPythonリソースと同じ場所のhelperを使います。出力先は`dist/RPG-Maker-Launcher.app`です。`--noconfirm`での再ビルドは既存の`dist/`内の同名成果物を更新します。`build/`、`dist/`、`.venv/`はGitの追跡対象外で、実ゲーム素材、ランタイム、検証画像などの大きな検証物もリポジトリには追加しません。

外部ランタイムのUniversalバイナリをARM側で使うことと、ランチャー自身のUniversal2配布は別です。ランチャーはarm64のみで、specにDeveloper ID署名は設定していません。PyInstallerがローカルで付けるad-hoc署名は、配布用署名や公証の代わりにはなりません。

Go移行時のビルド・テスト環境は、macOS 26.5.1 arm64、Python 3.11.2、Go 1.27.3、PySide6-Essentials 6.12.0、PyInstaller 6.22.3でした。直接のPython依存関係は`src/python/requirements-dev.txt`に固定しています（chardet 5.2.0はpure Python）。当時の検証用venvは`/tmp/rpgm-qt-venv`で、現在の手順の`.venv`とは別の環境です。arm64・x64・Universal2のhelperと、**arm64の.appを実際にビルド**しました。同梱helperはQProcess経由で公式の320リリースを返し、同梱GUIも隔離HOMEのoffscreen起動を3秒間行ってtracebackがないことを確認しました。これは通常の画面操作や配布先での起動保証とは区別しています。

mise切替時には、同じmacOS 26.5.1 arm64上でmise 2026.10.19、native Python 3.11.18、Go 1.27.3とプロジェクトの`.venv`を使い、既存の固定依存関係で39件すべてのPythonテスト（skipなし）、Goの`test`・`test -race`・`vet`、`pip check`、arm64のPyInstallerビルドが成功しました。`run`タスクは構成だけを確認し、ランチャーGUIや生成した.appの起動、実ゲームの再試験は行っていません。上記のMZ検証結果や320リリースの取得、3秒間の起動確認はPython 3.11.2を使った当時の記録であり、この切替で再確認した結果ではありません。

## ライセンス・出典・謝辞

元プロジェクトのライセンスを引き継ぎます。GNU GPL v3の全文は[LICENSE](LICENSE)にあります。外部ランタイム、ゲーム本体、素材、フォントなどには、それぞれの利用条件が適用されます。検証に使った公開デモのリポジトリにライセンスがあっても、含まれるRPG Maker素材などの再配布を一律に許可するものとは扱っていません。

ランチャー向けのカスタムMKXP-Z forkは[m5kro/mkxp-z の RPGM-Launcherブランチ](https://github.com/m5kro/mkxp-z/tree/RPGM-Launcher)です。以下は元プロジェクトのクレジットです。チートメニューと外部セーブエディターへの謝辞は元プロジェクトの旧機能に関する履歴であり、このforkではそれらの機能・導線を撤去しています。

### Credits and Special Thanks（元プロジェクト）

Donators:<br>
lecrolonk <br>
athanadoc05 <br>
<br>
Credit to mkxp-z maintainers for mkxp-z <br>
https://github.com/mkxp-z/mkxp-z <br>
<br>
Credit to emerladCoder for cheat menu <br>
https://github.com/emerladCoder/RPG-Maker-MV-Cheat-Menu-Plugin <br>
<br>
Credit to SaveEditorOnline for the save editor <br>
<br>
Credit to SynthFont developers for the Soundfont <br>
https://musical-artifacts.com/artifacts/841 <br>
<br>
Credit to Orochimarufan for the Kawariki Patches <br>
https://github.com/Orochimarufan/Kawariki <br>
<br>
Credit to EasyRPG team for EasyRPG Player<br>
https://easyrpg.org/player/downloads/#release-macos<br>
<br>

### Donations（元プロジェクトの案内）

以下は上流READMEに掲載されているEthereum Walletです。このforkの作者の寄付先ではありません。

`0x60F040444876EB5996DfA04CB8d8fc8D2aB96CF7`
