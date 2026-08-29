# sshc

sshc は、OpenSSH の設定を管理し、SSH 接続を実行するためのアプリケーションです。`~/.ssh/config` を独自形式へ変換せず、コメント、記述順、空白を保ったまま編集します。

バックエンドは `sshc engine` で起動し、Web UI と API を提供します。`sshc` を実行すると、起動中のエンジンの URL を表示し、利用可能な環境ではブラウザを開きます。

## インストール

macOS と Linux では Homebrew を利用できます。

```sh
brew install aida0710/tap/sshc
```

または、取得するスクリプトとバイナリを同じリリースへ固定して実行します。

```sh
SSHC_VERSION=v0.20.1 sh -c \
  'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/v0.20.1/install.sh | sh'
```

`main` 上の可変なスクリプトを直接実行せず、導入したい[リリース](https://github.com/aida0710/sshc/releases)のタグをURLと`SSHC_VERSION`の両方へ指定してください。

Windowsでは、GitHub Release上のPowerShellスクリプトを実行します。管理者権限は不要です。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/aida0710/sshc/releases/latest/download/install.ps1 | iex"
```

`%LOCALAPPDATA%\Programs\sshc`へインストールし、ユーザー`PATH`へ追加します。同じコマンドを再実行すると、公開済みの最新安定版へ更新します。

各 OS 向けのバイナリと Android APK は、[GitHub Releases](https://github.com/aida0710/sshc/releases) から手動でもダウンロードできます。デスクトップアプリ、パッケージ形式のWindowsインストーラ、macOS の app bundle は配布していません。

Homebrewまたはreceipt対応版の`install.sh`で導入したsshcは、同じ管理元へ処理を委ねて更新できます。手動配置やソースビルドは自動で置き換えません。

```sh
sshc update
```

## 基本的な使い方

最初にエンジンを起動し、別のターミナルから UI を開きます。

```sh
sshc engine      # フォアグラウンドでエンジンを起動
sshc             # URL を表示し、可能であればブラウザで開く
```

`sshc engine` はデーモン化しません。常駐させる場合は tmux、systemd、launchd などのプロセス管理機能を使用してください。

sshc はログイン時の自動起動を設定せず、systemd unit やスケジュールタスクも作成しません。自動起動は OS のプロセス管理機能で設定します。`sshc engine` をフォアグラウンドプロセスとして登録してください。

```sh
tmux new -d -s sshc 'sshc engine'
```

初回起動時は、資格情報を保存する vault を作成します。

```sh
sshc vault create
sshc vault unlock
```

マスターパスワードは Web UI または `sshc vault` から入力できます。CLI は対話端末からのみパスワードを読み取り、引数や環境変数からは受け取りません。

SSH 接続は CLI から直接開始することもできます。

```sh
sshc ssh                      # 一覧から接続先を選択
sshc ssh <接続先>             # 保存済みの設定を使用して接続
sshc ssh --list               # Host alias の一覧を表示
sshc ssh <接続先> --non-interactive -- <コマンド>
                              # 対話端末を開かずリモートコマンドを実行
sshc info <接続先>            # 実効接続先を表示（--json に対応、engine 不要）
sshc status                   # エンジンの状態を表示（--json に対応）
sshc update                   # Homebrew／install.sh経由の導入を更新
```

`sshc info` は実際の接続と同じ解決処理を通り、`Include`、`Match`、`ProxyJump`、既定の `Port 22`、接続文字コードを反映します。保存済みパスワードや鍵パスフレーズは読みません。`SetEnv` の値と `ProxyCommand` のコマンド文字列も、任意の秘密を含み得るため表示しません。

S3 互換ストレージとの同期は、起動済みの engine と解錠済みの vault を使用します。CLI が設定ファイルを直接復号したり、別の同期実装を持ったりはしません。

```sh
sshc sync setup               # 対話 TTY で接続先と資格情報を設定
sshc sync                     # 同期状態を表示（--json に対応）
sshc sync push                # 通常の条件付き push
sshc sync push --force        # 確認した remote ETag にだけ force push
sshc sync pull                # conflict／removal があれば適用せず拒否
sshc sync pull --force        # exact preview を remote authoritative で適用
sshc sync now                 # engine の同期巡回を今すぐ実行
sshc sync auto on             # engine の自動同期設定を永続化（off で停止）
```

`sync setup` は stdin と prompt 出力の両方が対話端末の場合だけ動作し、access key、secret key、sync key を引数・環境変数・ファイルから受け取りません。空の同期先では生成された sync key をその端末に一度だけ表示します。別端末の復元に必要なので、安全な保管先へその場で保存してください。

通常の push は remote が変化すると CAS で拒否されます。`push --force` も無条件上書きではなく、action token が確認時の exact ETag に結び付き、競合後の自動再確認・再試行をしません。通常の pull は conflict と removal を適用せず、`pull --force` も preview の ETag と revision が変化すれば拒否します。`--json` を持つ sync 操作は stdout に結果または安定した failure を一つだけ出します。mutation 後の通信切断などで `outcome_unknown` になった場合は自動再実行せず、`sshc sync` と remote の状態を確認してください。

SSH接続の文字コードは接続詳細で保存できます。UTF-8、Shift_JIS、EUC-JP、ISO-2022-JPに対応し、ブラウザのターミナル、`sshc ssh <接続先>`、`--non-interactive`で同じ設定を使用します。

Serial と Telnet は `~/.ssh/config` へ保存せず、その接続だけの指定として利用できます。どちらもエンジンの起動は不要です。対話接続は `Ctrl+]` で切断します。

```sh
sshc serial                  # device pathとUSB識別情報を表示
sshc serial --json           # 同じ一覧を安定したJSONで表示
sshc serial /dev/ttyUSB0 --baud 9600 --data-bits 8 --parity none --stop-bits 1
sshc telnet console.example:23 --connect-timeout 5s
sshc telnet legacy.example:23 --encoding shift_jis

# textを送り、正規表現に一致する応答まで待つ
sshc serial /dev/ttyUSB0 --non-interactive --expect 'router# ' --timeout 10s --json -- 'show version'

# 終了状態を返さない機器では、明示した時間だけ読み取ることもできる
sshc telnet console.example --non-interactive --require-output --read-for 2s --max-bytes 1048576 -- 'show status'
```

Serial の既定値は、既存の`serialctl`と同じ9600 8-N-1、改行はCRです。parityはnone／odd／even／mark／space、stop bitsは1／1.5／2を指定できます。bootloaderやMCUでは`--dtr on|off`、`--rts on|off`、`--break 500ms`も接続直後に適用できます。Telnet の既定ポートは 23、改行は CRLF です。`--line-ending none|cr|lf|crlf`でautomation時の送信終端を変更できます。現行のSerial backendはflow controlなしだけに対応し、`--flow rtscts`と`--flow xonxoff`は無視せずエラーにします。AndroidではUSB Host API用driverを同梱していないため、Serial CLI backendは利用できません。

複数段階の操作には、version 1のJSON scriptを使用できます。`sendEnv`は環境変数を実行時にだけ読み、受信した同じbyte列をtranscriptから`[REDACTED]`へ置換します。秘密値を`send`やコマンド引数へ直接書くとshell履歴やプロセス一覧に残り得るため、秘密には`sendEnv`を使用してください。

```json
{
  "version": 1,
  "steps": [
    { "expect": "login: " },
    { "send": "admin" },
    { "expect": "Password: " },
    { "sendEnv": "CONSOLE_PASSWORD" },
    { "expect": "router# ", "timeout": "15s" },
    { "send": "show version", "lineEnding": "cr" },
    { "readFor": "2s" }
  ],
  "onFailure": {
    "send": "\u0003",
    "lineEnding": "none",
    "timeout": "500ms"
  }
}
```

```sh
# CIのsecret storeなどでCONSOLE_PASSWORDを設定した状態で実行する
sshc serial /dev/ttyUSB0 --non-interactive --script ./console.json --json
```

`expect`はGoのRE2正規表現です。送信前の残留inputは破棄し、pattern一致後も既定120ms（`--settle`で変更）の静穏を確認するため、古いpromptや長い出力内のprompt風文字列を成功と誤認しにくくしています。`readFor`は時間が満了すれば受信0 byteでも成功するため、応答必須の用途では`--require-output`を併用してください。0 byteなら終了code 1とJSONの`failure.kind: "no_output"`になり、`bytesReceived`でも判定できます。

scriptの`onFailure`はmain stepが失敗した場合だけ、指定したbyte列を最大5秒の独立したtimeoutで送信します。上のCtrl+Cは例であり、すべての装置に安全な復旧文字ではありません。対象装置のCLI仕様を確認して明示した場合だけ使用してください。成功時には送信せず、JSONの`failureCleanup`は失敗時だけattempt結果を返します。復旧送信の失敗は元の失敗理由や終了codeを上書きしません。

scriptは最大1 MiB／128 step、patternは4 KiB、1回の送信は64 KiB、transcriptは既定1 MiB（最大16 MiB）に制限されます。`--timeout`は接続後のscript全体を制限し、既定10秒、最大5分です。stepの`timeout`はそのstepをさらに短く制限します。`readFor`はscriptの最終stepだけで使用でき、その時間より短い全体／step timeoutが先に来ればtimeoutになります。JSON出力はUTF-8でないtranscriptをbase64として返します。終了codeは成功0、接続・I/O・上限超過1、入力またはscript不正2、timeout 124、割込み130です。同一ユーザー権限の別processから環境変数を保護する機能はないため、値をshell commandへ直接書かず、CIのsecret storeや履歴に残らない入力方法で設定してください。

Telnetは暗号化もserver認証もしません。sshcは接続のたびに警告を表示しますが、通信を保護する機能は追加しません。資格情報を送る場合は、信頼できる隔離networkなど別の保護境界が必要です。

SSH alias は `sshc ssh` namespace の内側だけで解釈します。したがって `serial`、`telnet`、`status` などのコマンド名も、`sshc ssh serial` や `sshc ssh status --non-interactive -- <コマンド>` のように通常のaliasとして使用できます。transportを省略した `sshc <alias>` と、旧 `sshc run ...` は受け付けません。

引数なしの `sshc` はエンジンを起動しません。実行中のエンジンを置き換えるには `sshc engine --replace` を使用します。

## 主な機能

- 横断検索: desktop toolbarまたは`Ctrl/Cmd+K`から、保存済みhost、SSH設定file、snippet、設定画面を一つのCommand Paletteで検索して開けます。mobileではhamburger menu内から開き、terminalの表示領域を常時消費しません。
- OpenSSH 互換の設定管理: コメント、記述順、空白を保ったまま編集し、外部変更との競合を検出します。
- ブラウザ内ターミナル: ポート転送、エージェント転送、未知のホスト鍵の確認、切断時の自動再接続と終了後の明示再接続、`ProxyJump`、`ProxyCommand` に対応します。明示再接続は同じpaneとscrollbackを残したまま新しいshellを開きます。接続・再接続・終了を区別して表示し、利用者の確認が必要なホスト鍵や認証の問題は無条件に再試行しません。scrollback検索、live session内のcommand履歴、頻度順command候補、absolute remote path補完も利用できます。
- SFTP ファイル操作: 保存済み接続を使ったリモート閲覧、ファイル／フォルダの選択と Drag & Drop による再帰アップロード、ファイルdownload、フォルダのZIP download、作成、名前変更、chmod、削除に対応します。upload／download共通のTransfer Managerが同時2件までを実行し、file単位の進捗・速度・残り時間、pause／resume／retry／cancel、folder batchの失敗fileだけの再実行、別画面でのbackground転送と完了／失敗通知を提供します。large uploadは1 MiB chunkとremote part fileで通信断後のoffsetから再開してatomic renameし、file downloadはHTTP Rangeで再開します。UTF-8 の 2 MiB 以下のファイルは、遅延ロードされる Monaco Editor で競合を検出しながら編集できます。
- Workspace: 接続済みSSHターミナルまたはローカルシェルを、表示中のターミナルの上下左右へDrag & Dropすると、接続を増やさずLive Workspaceとして分割し、接続一覧でも1つのグループにまとめます。区切りをDragして比率を変更でき、単一paneへ集中するFocus Modeを利用できます。スマホでは分割を小さく並べず、Workspace内のターミナルをタブで1画面ずつ切り替えます。pane 構成は端末内に保存でき、Homeの一覧から1回の明示操作で再オープンできます。再オープン時は各SSH接続とローカルシェルを新しく開始し、一部が失敗しても成功したpaneを維持します。Command Centerはpreview後、接続中のSSH paneだけを対象に現在の入力先へcommandとEnterを送り、paneごとのcwd・環境・shell状態をそのまま使います。
- Snippets と automation: master keyで暗号化したライブラリへ`{{variable}}`を含むコマンドを保存し、接続先と展開後コマンドを確認してから複数ホストへ実行できます。明示的に選んだ snippet は、SSH shell の準備完了後に startup command として送信できます。
- 接続ログ: `ssh -v` 相当の情報を、4 段階の詳細度でターミナルに表示します。
- 表示設定: 6 種類のカラーパレット、同梱の JetBrains Mono、背景画像を全体または接続ごとに設定できます。
- SSH 鍵管理: 鍵の生成、パスフレーズ変更、ssh-agent への登録、リモートの `authorized_keys` への公開鍵追加に対応します。
- 資格情報 vault: パスワードと鍵パスフレーズをマスターパスワードで暗号化します。12 時間操作がない場合は自動的にロックされます。通常の一覧 API は保存値を返しません。名前や値を編集するときだけ、セッションと CSRF に加えて現在値へ結び付いた一度限りの確認トークンを要求する専用 API が 1 件を返し、応答はキャッシュしません。CLI で接続するときは、その接続経路に必要な資格情報だけを、ローカルの handoff secret で認証した経路から CLI へ渡します。
- S3 互換バックアップ: 暗号化したスナップショットを保存し、別の端末で復元できます。

Workspace の layout はプロセス状態や session ID を保存せず、この端末だけの `~/.ssh/sshc/workspaces.json` に置かれます。Snippets と startup binding は資格情報と同じ master key で暗号化され、remote sync の暗号化 snapshot にも含まれます。secret 型の値は通常previewでは伏せられますが、実行後はremote shell historyやterminal outputへ残る可能性があります。SFTP や複数ホスト実行は対話プロンプトを出さないため、vault の保存済み資格情報と既知のホスト鍵が必要です。

設計上の判断と対象外の機能については、[設計概要](docs/design.md)を参照してください。

## 開発

Go 1.26 と Node.js 22 が必要です。

```sh
make build       # Web UI をビルドし、bin/sshc を生成
make test        # Go と Web UI のテストを実行
make e2e         # 実バイナリに対する Playwright テストをローカル実行
make generate    # OpenAPI から Go と TypeScript のコードを再生成
```

S3 互換ストレージと OpenSSH に対する統合テストでは、先に Docker コンテナを起動します。

```sh
make integration-up
make integration
make integration-down
```

`internal/ui/dist` はリポジトリに含まれ、Go バイナリへ埋め込まれます。`web` 以下を変更した場合は `make build` を実行し、生成物もコミットしてください。CI でソースとの不一致を検出します。

Android AAR の生成には NDK が必要です。

```sh
go install golang.org/x/mobile/cmd/gobind
make android-bind
```

関連資料:

- [設計概要](docs/design.md)
- [手動受け入れ試験](docs/manual-acceptance.md)
- [エンジンの常駐例](docs/headless-examples.md)
- [インストールとアップグレード](docs/release-install.md)
- [リリース履歴](docs/releases/README.md)
- [文章と用語のガイド](docs/writing-style.md)

## ライセンス

[Apache License 2.0](LICENSE)
