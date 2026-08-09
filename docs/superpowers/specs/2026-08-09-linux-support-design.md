# Linux 対応と、Keychain の廃止

`GOOS=linux go build ./...` と `go vet ./...` は、いまそのまま通る。ビルドタグは
1 つもなく、`syscall.Exec` も `O_NOFOLLOW` も Linux にある。足りないのは
`internal/platform/macos` の中身が macOS のプログラムを呼ぶことだけである。

そして測ってみると、その 2,050 行のうち本当に macOS 固有なのは半分以下だった。

| ファイル | 行 | 固有部分 |
|---|---:|---|
| `command.go` | 148 | なし。純粋な `os/exec` |
| `keyagent.go` | 166 | `--apple-use-keychain` の 2 行だけ |
| `toolchain.go` | 57 | 探索ディレクトリの一覧のみ |
| `browser.go` | 45 | `/usr/bin/open` の定数 1 つ |
| `loginitem.go` | 125 | 全部（launchctl と plist） |
| `terminal.go` | 429 | 全部（AppleScript と Launch Services） |

## Keychain をやめる

`ssh-add --apple-use-keychain` は、パスフレーズをログインキーチェーンにも保存する
経路だった。これを廃止する。macOS からも消す。

理由は 3 つある。**保存の中核はすでに自前にある** — マスターパスワードで封じた
vault が、鍵のパスフレーズを名前付きの資格情報として持っている。Keychain はその
横に並ぶ二つ目の保管場所であり、同じことを二か所で覚えている。**修復できない
欠陥を抱えている** — Keychain の項目は絶対パス（`SSH: <path>`）で識別されるので、
鍵を移動すると壊れる。このアプリケーションは Keychain を読み書きしないため、
警告することしかできない。README がそう認めている。**Linux に相当物がない** —
`gnome-keyring` は `ssh-add` から直接は使えないので、残せばプラットフォームごとに
画面が違う状態が固定される。

消える面は `--apple-use-keychain` の 2 行だけではない。API の `storeInKeychain`
（リクエスト）、`storedInKeychain`（レスポンス）、`keychain_entry_stale`（注意）、
Keys 画面のチェックボックスとその文言、openapi.yaml、そしてそれらのテストである。

これにより `keyagent.go` は `ssh-add` を叩くだけの汎用コードになり、共有側へ移せる。

この廃止は Linux 対応より先に行う。`keyagent.go` を共有側へ移せるかどうかが、
次節の分け方に効くからである。

## 分け方

ビルドタグで分ける。`//go:build darwin` と `//go:build linux`。実行時の
`runtime.GOOS` 分岐にはしない。

macOS のバイナリに Linux のコードが、Linux のバイナリに AppleScript の定数が
入るのは、「出荷物に何が入るか」を気にするこのコードベースの姿勢と合わない。
`TestNoTestOnlyPackageReachesTheShippedBinary` が `internal/acceptance` について
言っているのと同じことを、プラットフォームについても言えるようにする。

```
internal/platform/          インターフェース。変更なし
internal/platform/process/  新設・共有。OutputRunner と KeyAgent
internal/platform/macos/    darwin。toolchain / browser / loginitem / terminal
internal/platform/linux/    linux。同じ 4 つ
cmd/sshc/wiring_darwin.go   darwin。部品を組み立てて返す
cmd/sshc/wiring_linux.go    linux。同上
```

`process` へ移すのは `command.go` をそのまま、`keyagent.go` を Keychain の 2 行を
抜いた形で。どちらも移動であって書き直しではないので、テストも一緒に移す。

`Toolchain` は機構と一覧を分ける。ディレクトリの列を持って `Stat` で探すという
中身は共有し、その列だけがプラットフォームごとに違う。

## Linux 側

**Toolchain**: `/usr/bin`、`/usr/local/bin`、`/bin`。macOS と同じく PATH は
参照しない。理由も同じで、このアプリケーションが実行するプログラムが、継承した
環境に依存してはならない。

**Browser**: `/usr/bin/xdg-open`。macOS 版と同じく絶対パスで、ループバックの http
以外は拒否する。URL は生きた bootstrap トークンを運ぶ。

**LoginItem**: `~/.config/systemd/user/sshc.service` を書き、`systemctl --user
enable --now` で有効にする。macOS 版と同じく `-open=false` で起動し、**標準出力を
どこにもリダイレクトしない**（`StandardOutput=null`）。あの出力には有効な
bootstrap トークン付きの URL が乗るので、journald はその置き場所ではない。代わりに
`sshc open` がその場で新しい入口を発行する。`systemctl` が見つからない環境では、
このコントローラを組み立てず、設定を未対応として報告する。`LoginItemController`
が nil を許す仕組みは既にある。

**Terminal**: 実装しない。`platform.TerminalLauncher` を一切組み立てず、
`s.Terminal` は nil のままにする。`TerminalCustom` すら持たない。

理由は表の話ではない。`platform.OutputRunner.RunOutput` は子プロセスの終了を
待つ。端末エミュレータをこの経路で直接起動すれば、SSH セッションが続く間
HTTP リクエストが開いたままになり、リクエストがキャンセルされれば端末は
SIGKILL される。macOS はこれを免れている——起動しているのは `/usr/bin/open`
であり、対象のアプリケーションを起こしてすぐに戻るからだ。Linux の端末には
その相当物がない。利用者は、画面が示すコマンド文字列（このバイナリと alias）を
自分の端末で実行する。

## 権限とパス

`0o600` と `0o700`、`O_NOFOLLOW`、`~/.ssh` の位置、`syscall.Exec` は、Linux では
macOS と同じ意味を持つ。storage 層に変更はない。

`%u` と `%i` も同じである。`os/user` が返すユーザー名と uid は Linux でも数値の
uid になる。

## Windows をやらない理由

`O_NOFOLLOW` がない。`syscall.Exec` がない。`0o600` が意味を持たない
（Win32-OpenSSH は ACL で検査する）。この 3 つはいずれも、このコードベースが明文で
依拠している性質である。「シンボリックリンクを通して書かない」「端末を ssh に
引き渡すのであって子プロセスを挟まない」「~/.ssh の権限が締まっている」。作り替える
なら README の主張も書き直すことになるので、別作業とする。今回のビルドタグ分割は
その足場にはなる。

## テスト

移動したものは、テストも一緒に移す。中身は変えない。

Linux 版の 3 つ（Toolchain、Browser、LoginItem）は macOS 版と同じ形で試す。
記録用のランナーが argv を受け取り、**プロセスは一切起動しない**。このリポジトリの
どのテストも、本物のブラウザ、本物の systemd、本物の端末には触れない。

CI は既にすべてのジョブが `ubuntu-latest` で動いており、macOS のジョブは
一つも無かった。ビルドタグで分けた結果、`internal/platform/macos` は ubuntu の
ランナーでは一行もコンパイルされず、そのテストは静かに CI から消えていた。
だから足すのは ubuntu のジョブではなく、`macos-14` で `go vet`、`go build`、
`go test`、`go test -race` を走らせる macOS のジョブである。E2E は
`ubuntu-latest` のまま変えない——Playwright が駆動するのは埋め込み UI であって、
プラットフォーム層ではないからだ。

`make build` は変えない。出力は `bin/sshc` のままで、Linux でビルドするときは
`GOOS` が既に環境から決まっている。リリースワークフローも変えない。Linux の
配布物を出すかどうかは別の判断であり、この作業には含めない。

## 境界

- ログイン時起動は systemd がある環境で動き、ない環境では未対応と報告する。これは
  macOS で `LoginItem` が nil のときと同じ振る舞いであり、Linux だけの欠落ではない。
- **Linux は端末を起動しない。** `platform.OutputRunner.RunOutput` は子プロセス
  の終了を待つため、端末エミュレータをこの経路で直接起動すれば、SSH セッションが
  続く間 HTTP リクエストが開いたままになり、キャンセルされれば端末は SIGKILL
  される。macOS はこれを免れている——起動しているのは `/usr/bin/open` であり、
  対象のアプリケーションを起こしてすぐに戻るからだ。Linux の端末にはその相当物が
  なく、これは systemd の有無のような環境差ではない、Linux 全体で欠けている機能
  である。利用者は画面が示すコマンド文字列を自分の端末で実行する。
- Keychain は、どのプラットフォームにも存在しない。
