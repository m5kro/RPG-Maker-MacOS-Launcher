<img src="icon.png" height="200" alt="RPG Maker Launcher icon">

# RPG-Maker-MacOS-Launcher

RPG Maker製のゲームをmacOSで動かすために、実行環境の導入とゲームの起動をまとめて扱うランチャーです。ゲーム制作ツールそのものではなく、手元にあるゲームを、その形式に合ったmacOS向けランタイムで起動します。MV・MZにはNW.js、XP・VX・VX AceにはMKXP-Z、2000・2003にはEasyRPGを使う構成を元プロジェクトから引き継いでいます。すべてのゲームやプラグインで動作することを保証するものではありません。

このforkは、[m5kro/RPG-Maker-MacOS-Launcher](https://github.com/m5kro/RPG-Maker-MacOS-Launcher)をもとに、安全性の見直しと段階的なGo移行を進めている派生版です。上流の「開発完了、必要に応じてバグ修正のみ」という案内は元プロジェクトについてのもので、このforkの作業完了を意味しません。上流では、より汎用的な後継として[Xenolauncher](https://github.com/m5kro/Xenolauncher)も案内されています。

今回Goに移したのは、**NW.jsのバージョン一覧取得と、検証付きインストールだけ**です。画面は引き続きPython/PySide6で動き、ゲームの起動・書き出し、パッチ、チート機能、MKXP-Z/EasyRPGの取得もPython側に残っています。全体をGoで書き直したわけではなく、Goを使うことでNW.jsなどのゲーム用ランタイムが不要になるわけでもありません。

> [!WARNING]
> 現時点では配布用の署名・公証を保証していません。実機検証に使ったNW.jsは、リソースの署名封印がないため厳密な`codesign`検証に失敗しました。このホストではセキュリティ回避操作なしで起動しましたが、別のMacやFinder/Gatekeeper経由で起動できることの証明にはなりません。OSが起動を拒否した場合は、保護機能を無効にして続行しないでください。

## なぜ、まずNW.jsの取得処理をGoに移したのか

ゲームを起動する前のダウンロードには、通信待ち、検証、展開、キャンセル、既存版との共存が伴います。今回はこのまとまりを小さな独立プログラムに切り出し、画面を待たせずに処理しながら、失敗時の扱いを明確にすることを優先しました。GUIまで同時に作り直すのではなく、既存の画面を残して境界のはっきりした部分から移行しています。ゲームの速度向上や、互換性の拡大を実証した変更ではありません。

`Install NWJS Version`を押すと、Qtの`QProcess`がGo helperの`versions`を非同期で起動し、選択後に`install`を実行します。ダウンロード・検証・展開の進捗はJSONLで画面に伝えます。正常終了と期待する完了通知の両方を確認してから、その版を選択して設定を保存します。helperが見つからない場合に、旧Python版の同期ダウンロードへ戻る処理はありません。

導入するNW.jsはネイティブのアーキテクチャを優先します。Apple Siliconでx64版しかないリリースを選ぶ場合は、Rosetta利用への明示的な同意が必要です。ただし、今回の実機確認はarm64に限られ、Rosettaでの動作は未検証です。

### ダウンロードと既存版の扱い

helperは公式の`SHASUMS256.txt`とダウンロードしたアーカイブのSHA-256を照合してから展開します。チェックサムの欠落、不正なアーカイブ、通信エラー、アーキテクチャの不一致があれば、そのまま導入を進めずエラーにします。作業場所は`~/Library/Application Support/RPGM-Launcher`内の専用の非公開一時ディレクトリ（`.nwjs-stage-*`）です。導入完了までは作業用ディレクトリを版一覧に出さず、一覧には実行可能な`nwjs.app/Contents/MacOS/nwjs`がある版だけを表示します。

失敗したからといって、既存版を削除したり置き換えたりはしません。helperで導入済みの同じ版は再検証し、条件が一致すれば再ダウンロードせずに完了します。一方、旧ランチャーが導入した版、不完全な版、条件の合わない同名ディレクトリは、勝手に上書きせず残します。置き換えたい場合は先にバックアップし、正常な旧版なら明示的にアンインストールしてください。一覧に出ない不完全なディレクトリは、内容の手動確認が必要になることがあります。

キャンセル時はまずSIGTERMで後片付けを促し、3秒以内に終了しなければSIGKILLに切り替えます。ウィンドウを閉じる操作でも、画面のイベント処理を止めずにhelperの終了を待ちます。一覧取得・導入中は、起動・書き出し・アンインストールなどの管理操作を無効にします。ただし強制終了や電源断では、一時ディレクトリや予約済みの空の版ディレクトリが残る場合があります。また、導入先への配置後にキャンセルすると、有効な版が残っても自動選択されないことがあります。再起動すれば一覧に現れますが、キャンセルによって配置済みの版が巻き戻されるわけではありません。

Go側の導入とPython側のNW.js削除・`Uninstall All`は、OSの`flock`で同じ`.nwjs-install.lock`を使い、競合する操作を待たずに拒否します。`Uninstall All`でも管理ルートとロックファイルの実体は残します。**ランチャーやhelperの動作中に、このロックを削除しないでください。** この取り決めを知らない旧ランチャーや外部ツールとは排他できないため、同時実行は避けてください。残った作業ディレクトリを調べるのも、すべてのインストーラーを停止してからにしてください。

## ゲーム原本を守るために変えたこと

ゲームフォルダーを選んだだけでは、`package.json`を書き換えたり、ゲーム名に対応するユーザープロファイルを削除したりしないようにしました。起動時の共有NW.jsプロファイル削除、ゲームフォルダー全体への再帰的な`chown`/`chmod`、設定のバージョン違いを理由にした設定一括削除も撤去しています。ゲーム独自のバックグラウンドスクリプトは保持し、保護用スクリプトと衝突した場合も勝手に置き換えません。原本内のWindows向けファイルを削除していた`Optimize Space`は無効です。

これは、ランチャーのすべての操作が非破壊になったという意味ではありません。起動時のメタデータ補修やパッチ、チート追加、展開コピーには、ゲームファイルを書き換える経路が残っています。`Extract game_en.exe`は元のEXEこそ残しますが、ほかのファイルを上書きし、途中で失敗しても変更を巻き戻せません。**ゲームとセーブをバックアップし、試すときは専用のコピーを使い、`Extract game_en.exe`をOFFにしてください。** 信頼できないゲームを安全に隔離するOSサンドボックスでもありません。

## どこまで確認できているか

今回の検証では、Python/Qt連携23件と安全性16件の計39テスト、およびGoの`test`・`test -race`・`vet`が成功しました。Qt連携は実際のイベントループと偽の実行可能helperを使い、分割されたUTF-8/JSONL、末尾改行なし、一覧取得から導入への遷移、起動失敗、異常終了、不正・欠落した完了通知、Rosetta同意、キャンセルと強制終了、終了待ち、操作の無効化、成功時だけの更新を確認しています。ビルド済みの実Go helperとPythonの間で、同じロックが効くことも確認しました。

実機では、Apple Silicon上でGo helperからNW.js SDK 0.94.1 arm64を取得・検証・導入し、公開MZ 1.8.0デモ1本で、タイトルからNew Game、移動、イベント、メニューでの保存、プロセス終了と再起動、Continueからのロードまでを2回確認しました。操作はCDPによるキーボードイベントで、エンジンの保存・ロードAPIを直接呼んだ結果ではありません。原本1,332ファイルは不変でした。ゲームの出典・固定コミット、ハッシュ、画像や状態JSONの記録先、署名失敗の詳細と再現手順は、[検証記録と制約](docs/validation/nwjs-go-migration.md)にまとめています。

このゲーム試験はhelperが導入したランタイムを直接起動したものです。**ランチャーの`Start Game`、パッチ、書き出しまで通した実ゲーム検証ではありません。** 別途行った実Qt→Goの接続試験も、導入済みの同じ版を再検証して完了する経路であり、GUIから初回ダウンロードを最後まで通す試験ではありません。

音声が実際に聞こえること、ほかのMZ作品やプラグイン、MVやそれ以外のエンジン、Intel/Universal2での実行、Rosetta、長時間動作は未検証です。MKXP-Z/EasyRPGや書き出しなどの既存機能はコードに残っていますが、今回のGo移行で動作保証が追加されたわけではありません。配布用署名・公証とGatekeeper/Finder経由の挙動も未確認です。

## ソースから試す・ビルドする（macOS）

このforkのソースを取得し、リポジトリのルートで以下を実行してください。[上流のビルド済みリリース](https://github.com/m5kro/RPG-Maker-MacOS-Launcher/releases/latest)は元プロジェクトの配布物であり、ここで説明しているGo移行を含むものではありません。

必要なのはPython **3.11**、Go **1.25以上**、Xcode Command Line Tools（`lipo`を使用）です。`requirements-dev.txt`で固定したQt 6.12のwheelは**macOS 14以上**を必要とします。古いmacOSでは別のQt・依存関係の組み合わせを検証する必要があります。Pythonの依存関係は仮想環境に入れるため、システム全体へのpipインストールは不要です。Goはhelperのビルドに使い、完成したアプリにはhelperの実行ファイルを同梱します。

```sh
python3.11 -m venv .venv
.venv/bin/python -m pip install -r requirements-dev.txt

# CGO_ENABLED=0でdarwin/arm64・darwin/amd64をビルドし、lipoでUniversal2も作成。
# 出力のアーキテクチャを検査し、無視対象のbuild/に保存します。
sh scripts/build-nwjs-helper.sh

# ソースからGUIを起動します。
.venv/bin/python main.py
```

MV・MZを試す場合は、画面の`Install NWJS Version`からゲームに合う版を導入し、`Select Game Folder`でバックアップ済みのゲームコピーを選びます。通常はその後に`Start Game`で起動する流れですが、前述のとおり今回の実ゲーム検証はこのボタン経由ではありません。パッチなどの設定を確認し、原本ではなく検証用コピーで試してください。元の操作説明として、[xem](https://forums.rpgmakerweb.com/index.php?members/xem.159435/)/[pgm](https://www.youtube.com/channel/UCWTsqPj8SD8rHL9gsBZI59Q)による[解説動画](https://www.youtube.com/watch?v=NIRiKIjhXHk)もありますが、このforkの変更前の案内です。

ソース実行時は、実行中のPythonのアーキテクチャに応じて`build/helpers/arm64/nwjs-helper`または`build/helpers/x64/nwjs-helper`を使います。探索の基準はプロジェクトの`__file__`からの絶対パスで、`PATH`ではありません。helperがない、または実行できない場合は、ビルドコマンドを含むエラーになります。

### テストとアプリ化

Python/Qtの通常テストはネットワークや実ゲームを使わず、一時HOMEと偽helperで実行できます。Pythonと実Go helperのロック連携テスト1件だけは、ネイティブhelperが未ビルドだとスキップされるため、先に上のビルドを済ませてください。実ゲームによる追加検証は別途[検証記録の手順](docs/validation/nwjs-go-migration.md#reproduce-safely)に従い、専用の作業場所と利用条件を確認したゲームコピーで行います。

```sh
QT_QPA_PLATFORM=offscreen .venv/bin/python -m unittest discover -s tests -v
go test ./...
go test -race ./...
go vet ./...

# Python/Qtのアーキテクチャに対応するspecを選びます。以下はarm64の例です。
.venv/bin/python -m PyInstaller --noconfirm RPG-Maker-Launcher-arm64.spec
# .venv/bin/python -m PyInstaller --noconfirm RPG-Maker-Launcher-x64.spec
# .venv/bin/python -m PyInstaller --noconfirm RPG-Maker-Launcher-Universal2.spec
```

各specは`build/helpers/{arm64,x64,universal2}/nwjs-helper`の対応する実行ファイルを、データではなくバイナリとして同梱します。アプリ内では、同梱されたPythonリソースと同じ場所のhelperを使います。出力先は`dist/RPG-Maker-Launcher.app`です。`build/`、`dist/`、`.venv/`はGitの追跡対象外で、実ゲーム素材、ランタイム、検証画像などの大きな検証物もリポジトリには追加しません。

Universal2版には、[Universal2のPython](https://www.python.org/downloads/macos/)と、それに対応するネイティブ依存ライブラリが必要です。Go helperだけを両アーキテクチャ対応にしても、単一アーキテクチャのPython/QtがUniversal2になるわけではありません。x64版にも対応するPython/Qtが必要です。specにDeveloper ID署名は設定していません。PyInstallerがローカルで付けるad-hoc署名は、配布用署名や公証の代わりにはなりません。

今回ビルド・テストに使った環境は、macOS 26.5.1 arm64、Python 3.11.2、Go 1.27.3、PySide6-Essentials 6.12.0、PyInstaller 6.22.3です。直接のPython依存関係は`requirements-dev.txt`に固定しています（chardet 5.2.0はpure Python）。実際の検証用venvは`/tmp/rpgm-qt-venv`で、上のコマンドの`.venv`に相当します。arm64・x64・Universal2のhelperと、**arm64の.appを実際にビルド**しました。同梱helperはQProcess経由で公式の320リリースを返し、同梱GUIも隔離HOMEのoffscreen起動を3秒間行ってtracebackがないことを確認しました。これは通常の画面操作や配布先での起動保証とは区別しています。

## ライセンス・出典・謝辞

元プロジェクトのライセンスを引き継ぎます。GNU GPL v3の全文は[LICENSE](LICENSE)にあります。外部ランタイム、ゲーム本体、素材、フォントなどには、それぞれの利用条件が適用されます。検証に使った公開デモのリポジトリにライセンスがあっても、含まれるRPG Maker素材などの再配布を一律に許可するものとは扱っていません。

ランチャー向けのカスタムMKXP-Z forkは[m5kro/mkxp-z の RPGM-Launcherブランチ](https://github.com/m5kro/mkxp-z/tree/RPGM-Launcher)です。以下は元プロジェクトのクレジットです。

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
https://saveeditor.online <br>
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
