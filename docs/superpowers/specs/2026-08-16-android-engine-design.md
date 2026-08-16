# Android 対応 (1) — エンジンをアプリの中で動かす

`GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` は、いまそのまま通る。
ビルドタグを 1 つも足さずに通る。Go では `GOOS=android` が `unix` タグも `linux`
タグも満たすので、`pty_unix.go` も `enginelock_unix.go` も `wiring_linux.go` も、
すでに Android 向けにコンパイルされている。

壁は言語ではない。**engine を起こす形と、engine が居る世界の前提**である。

| 前提 | Android での実態 |
|---|---|
| 外殻が engine を子プロセスとして起こす | プロセスは 1 つ。子を持つと low memory killer が黙って殺す |
| `os.UserHomeDir()` がユーザーのホームを返す | `HOME` は当てにならない。ホームは `context.filesDir` |
| `~/.ssh` に OpenSSH の設定がある | OpenSSH が無い。設定はこのアプリだけのもの |
| `/bin/bash` が居る | 居ない。`/system/bin/sh` (mksh) だけ |
| `ssh-keygen` と `ssh-agent` が居る | 居ない |
| バイナリを置き換えて自己更新できる | できない |
| pure-Go リゾルバが名前を引ける | **引けない。`/etc/resolv.conf` が無い** |

最後の行が方式を決める。Android には `/etc/resolv.conf` が無く、名前解決は netd を
通る。つまり cgo リゾルバが要り、`CGO_ENABLED=1` と NDK が確定する。冒頭の
`CGO_ENABLED=0` のビルドが通ったのは、コンパイルが通るという以上のことを何も
意味していない。

## 方式 — gomobile bind

`mobile` パッケージを 1 つ公開し、`gomobile bind -target=android` で AAR を吐く。
Kotlin からは普通の Java メソッド呼び出しになる。

退けた案を 2 つ記録しておく。

**`-buildmode=c-shared` + 手書き JNI。** x/mobile への依存が消える代わりに、
C を 80 行抱える。渡す型が `string` と `error` しか無い今回の API では、gomobile
の型制約が問題にならないため、C を書く理由がない。gomobile が Go 1.26.6 で動かな
かった場合の退避先として残す。

**engine を子プロセスで起こす（Electron と同じ形）。** APK の
`nativeLibraryDir` に置いたファイルは exec できる（W^X が塞ぐのはアプリの書き込み
可能なデータディレクトリからの実行だけ）ので、`cmd/sshc` を
`lib/arm64-v8a/libsshc_engine.so` として同梱すれば、Go 側の差分はほぼゼロで済む。
退けたのは、その子プロセスが Android のライフサイクルの外に居るからである。
foreground service で生かすにしても、サービスと同一プロセスに置けるものを
わざわざプロセス境界の向こうに置くことになる。加えて「CLI を削除」が名目だけに
なる——バイナリの中に全サブコマンドが残る。

## `mobile` パッケージ

リポジトリ直下に置く。`internal/` の下では gomobile が bind できない。

```go
func Start(home, cache string) (string, error)  // bootstrap URL を返す
func Stop() error
func Version() string
```

パッケージレベルの singleton を mutex で守る。**構造体を bind して Kotlin に
インスタンスを持たせない。** 1 プロセスに engine は 1 台という制約は、Android では
設計判断ではなく事実である。Kotlin 側に複数持てる形を見せれば、持てないものを
持てるように見せることになる。

中身は `cmd/sshc/engine.go` の `runEngineApp` を写したものになる。落とすのは 4 つ。

- `ownershipMonitor` と stdin の EOF 監視。**親が死ねば engine も死ぬ**のが同一
  プロセスでは自明なので、監視する対象が存在しない。
- シグナル処理。Android はプロセスにシグナルを送って落としたりしない。
- 終了コードへの写像。返すのは error であって終了コードではない。
- `Updates`（`selfupdate.Checker`）。nil にする。バイナリを置き換える自己更新は
  Android に無い。

残すのは `enginelock` である。Activity の再生成で 2 台目が起きないことの保証は、
Android でも要る。

`handoff.Owner` は `OwnerDesktop` を再利用する。**新しい owner 値を足さない。**
欲しいのは「bootstrap fragment 付きの URL を `Announce` が返す」という既存の
desktop の挙動そのものであり、名前を増やせば `handoff` の検証と `app.Run` の分岐に
波及する。

`Announce` は URL をチャネルへ渡し、`Start` は「`Announce` が来る」か「`Run` が
落ちる」かのどちらかまでブロックする。

ログは `slog.New(slog.NewTextHandler(log.Writer(), …))` にする。gomobile bind が
`golang.org/x/mobile/internal/mobileinit` を通じて標準 `log` の出力先を logcat へ
差し替えるので、cgo を 1 行も書かずに logcat へ流れる。

## プラットフォーム継ぎ目

**`internal/platform/shell.go`。** `shellFallbacks()` に android を足し、
`/system/bin/sh` を返す。現状 `runtime.GOOS` を関数の中で直接読んでいるので
テストが書けない。`shellFallbacks(goos string)` に引数化する。

ローカルシェルを残すのは意図的な選択である。Termius も JuiceSSH も ConnectBot も
ローカルシェルのタブを持たない。ローカルシェルを持つ Termux は、自前の userland を
`/data/data/com.termux/files/usr` へ展開し、Android 10 以降の W^X を避けるために
targetSdkVersion 28 に固定して実現している——現行 SDK を target するアプリには
取れない手である。ここで開くのは `/system/bin/sh`、つまり mksh + toybox の箱庭で、
見えるのはアプリの私有ディレクトリだけになる。**それでよいと決めた。**

**`Environ`。** `os.Environ` を渡さない。アプリの環境変数に有用な `PATH` は
入っていない。固定の 4 本を渡す。

```
HOME=<filesDir>
PATH=/system/bin:/system/xbin
TERM=xterm-256color
TMPDIR=<cacheDir>
```

**`Toolchain` は空、`KeyAgent` は nil。** `app.Dependencies` のコメントが既に
「`KeyAgent` が nil の場合、エージェント登録は到達できるエージェントがないと報告
する」「`Toolchain` は、見つかるかどうかで、ハードウェア鍵の項目を一覧に出して
よいかを決める」と約束している。Android は、その約束が初めて本番で使われる場所に
なる。**約束されているだけで表明されていないので、表明するテストを足す。**

**`cmd/sshc/wiring_linux.go` のビルドタグは触らない。** `!android` を足せば
`newPlatformParts` が android から消え、`GOOS=android go build ./...` という CI
ゲートそのものが壊れる。CLI の削除は「APK に実行ファイルを入れない」ことで達成
する。Android 向けにコンパイルが通ること自体は、ゲートとして価値がある。

## Android 側

`android/` に Gradle プロジェクトを置く。

**`MainActivity`** — WebView 1 枚。`Sshc.start()` が返した URL を `loadUrl` する。
戻るキーは `webView.canGoBack()` に繋ぐ。`web/src/routing/` が既にセクションを URL
として持っているので、履歴はそのまま Android の戻るキーの意味になる。

**`EngineService`** — foreground service。**これは削れない。** SSH クライアントで
「パスワードをコピーしに別アプリへ行ったらセッションが切れる」のは、起きるかも
しれない話ではなく最初に起きる話である。`foregroundServiceType` は `dataSync`。

**`network_security_config.xml`** — `127.0.0.1` にだけ cleartext を許可する。
`android:usesCleartextTraffic="true"` は使わない。全ホストが開く。

**minSdk 26 / targetSdk は現行。** ABI は `arm64-v8a` と、エミュレータと CI の
ための `x86_64`。

## 起動の流れ

```
Application.onCreate
  └ EngineService 起動 → Sshc.start(filesDir, cacheDir)
        └ [Go] enginelock 取得 → app.Run → Announce(URL) → チャネル
  └ URL を受けて MainActivity が WebView.loadUrl(URL)
        └ POST /api/v1/session/bootstrap （fragment の消費。既存のまま）
```

`Home` が `filesDir` になるので、設定は `<filesDir>/.ssh/config` に置かれる。
これは端末の OpenSSH の設定ではなく、このアプリ専用の設定である。母艦の設定を
持ち込む経路は**既にある `remotesync`（S3）**であり、ここに新しい仕組みを作らない。

## エラー

`Start` が返す error は 3 つに畳む。ロックが取れない（既に起動している）、
listen できない、`Run` が即死。どれも WebView を出さず、Kotlin 側がネイティブの
エラー画面を出す。

**Go の error 文字列をそのまま画面に出さない。** bootstrap fragment を含み得る値が
UI と logcat に漏れる経路になる。畳んだ 3 つの区別だけを Kotlin へ渡す。

## テスト

`mobile` の組み立てを `newDependencies(home, cache string) app.Dependencies` として
切り出し、ホスト（darwin）上で表明する。

- `Updates` が nil であること
- `Toolchain` が空であること
- `KeyAgent` が nil であること
- `Environ` が固定の 4 本を返し、`os.Environ` を含まないこと
- `Start` の二重呼び出しが 2 台目を起こさないこと
- `Stop` の後に `Start` が再び成功すること

`shellFallbacks("android")` が `/system/bin/sh` を返すことを `shell_test.go` に
足す。

`Toolchain` が空・`KeyAgent` が nil のとき、ハードウェア鍵とエージェント登録の
経路が閉じることを `internal/httpserver` の既存スイートで表明する。

CI ゲートは `GOOS=android GOARCH=arm64 CGO_ENABLED=1 CC=<NDK clang> go build ./...`。
gomobile bind と APK ビルドは別ジョブ（サブプロジェクト 3）。実機での確認は当面
`docs/manual-acceptance.md` に手順として書く。

## 計画の最初に潰す 4 つの未知数

実装の順序ではなく、**方式が成立するかどうか**を決める。どれかが倒れたら設計へ
戻る。

1. **gomobile bind が Go 1.26.6 で動くか。** 倒れたら c-shared + 手書き JNI へ。
2. **cgo リゾルバで名前が引けるか。** 倒れたら、この設計は Android で SSH できない
   ことになる。最優先。
3. **`creack/pty` が Android の `/dev/ptmx` で動くか。** 倒れたらローカルシェルを
   落とす（リモートセッションは PTY を使わないので影響しない）。
4. **WebView から `ws://127.0.0.1:PORT` が CSP `connect-src 'self'` の下で通るか。**
   倒れたら埋め込みターミナルが動かない。

## スコープ外

生体認証によるロック解除。Android Keystore への vault 鍵の移設。SAF による設定
ファイルの取り込み。Play Store 配布と署名。自己更新。タブレットの分割画面。
