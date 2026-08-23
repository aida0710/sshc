# 手動受け入れ試験

自動テストが決して行わない操作をここに集めます。実リモート接続、実 `authorized_keys` 変更、実 ssh-agent への登録、そして実機の Android（通知からの停止、WebView の挙動）は自動化しません。

実施前に必ず読むこと。

- 実施は使い捨ての `HOME` で行い、本番の `~/.ssh` では行いません。
  `work="$(mktemp -d)"; cp -R ~/.ssh "$work/.ssh"; HOME="$work" ./bin/sshc`
- **`./bin/sshc` はブラウザを開きません。** 入口の URL を標準出力へ 1 行出すので、それを自分で開きます。
- 本番の `~/.ssh` を使う項目（ssh-agent とデスクトップ）は、事前に `~/.ssh` と `~/.ssh/sshc` を別ディレクトリへ退避してから行います。
- 各項目に日付、OS と OpenSSH のバージョン、結果を記録します。M3 は macOS でも Linux でも実施できます。

## M1. 実リモートホストへの接続テスト

1. 使い捨て `HOME` に、自分が管理する検証用ホストの `Host` ブロックを作る。
2. Diagnostics タブで「到達性」を実行し、`ProxyJump は使用していない` という注記が出ることを確認する。
3. 「認証テスト」を実行する。
4. 期待: 認証成功が報告され、**通った認証方式が名指しされる**（`publickey` など）。説明は 8 KiB 以内に切り詰められ、鍵本文もパスフレーズも表示されない。
5. 実行可能ディレクティブを持つ設定では、確認ダイアログに実際のコマンド文字列が表示され、確認するまで開始しないことを確認する。
6. Known Hosts パネルで「Scan」を実行し、候補が「unverified」と表示されること、別経路で得た fingerprint を入力するか明示的に承認するまで「Add to known_hosts」が押せないことを確認する。**実際にそのアドレスへ接続する**ため、この確認は自動テストに含めていません。
7. **接続を開き、端末の中で実際に使う。** `vim` を開いてウィンドウを広げ、描画が追随すること（`window-change` が届いている）。`LocalForward` を書いた設定で接続し、一覧に転送が出ること、そのポートへ繋ぐと向こう側に届くこと、コンソールを閉じるとポートが空くことを確認する。
8. **未知のホストへ接続し、端末に fingerprint を伴う問いが出ること**、`yes` と打つと繋がり `known_hosts` に 1 行増えること、`no` では繋がらず何も書かれないことを確認する。既知のホストの鍵を書き換えてから接続し、**尋ねられずに断られる**ことを確認する。

## M2. 実 `authorized_keys` への公開鍵登録

1. 検証用リモートホストに、削除してよい検証用ユーザーを用意する。
2. Remote Keys パネルで alias と公開鍵行を入力し、「Show what this would do」を押す。対象 alias、実効ユーザー、宛先、fingerprint、追記される 1 行、リモートで実行される固定スクリプトが表示されることを確認する。ここまでは自動 E2E でも確認しています。
3. 「Register the key」を実行し、リモートの `~/.ssh` が `0700`、`authorized_keys` が `0600` になっていることを `ls -l` で確認する。
4. 同じ鍵をもう一度登録し、`already_present` として重複行が増えないことを確認する。
5. POSIX shell を持たないリモート（例: 制限付きシェル）に対しては、手順の表示のみになることを確認する。この判定は実際に接続してみるまで分からないため、事前に警告する手段はありません。
6. 登録した行をリモートから削除して原状復帰する。

## M3. 実 ssh-agent

1. 本番の `~/.ssh` を退避したうえで実施する。
2. Keys 画面で鍵の行の「Add to agent」を押し、ライフタイムを選んで登録し、`ssh-add -l` に現れることを確認する。
3. 画面の ssh-agent 節が、`ssh-add -l` と同じ fingerprint を同じ数だけ表示していることを確認する。
4. `ssh-add -d <path>` で原状復帰する。
5. **`ssh-add` のプロセスが一つも現れないことを確認する。** 登録中に `ps -ax | grep ssh-add` を見る——このアプリケーションは agent のプロトコルを直接話すので、あれを起こしません。鍵の復号はこのプロセスの中で行われ、agent が受け取るのは復号済みの鍵です。

自動テストは `x/crypto/ssh/agent` のキーリングを相手に本物のプロトコルで往復します（`internal/keys/agent_test.go`）。実際のユーザーの agent に対して成立することを示せるのはこの手動試験だけです。

## M4. エンジンと入口

入口の発行、二台目の拒否、`.xterm` の描画は e2e が自動で見ます。**ここでしか
確かめられないのは、実際のブラウザとデスクトップ環境の振る舞いです** ——
`xdg-open` / `open` がどのブラウザへ渡すか、そのブラウザが loopback の origin を
どう扱うか、画面の無い機械で何も起こらないこと。

1. `sshc engine` を端末で起こし、**入口の URL が出ないこと**を確認する。次に打つべき
   `sshc vault ...` だけが出る。
2. 別の端末で `sshc` を打ち、URL が刷られ、**既定のブラウザがそれを開く**ことを
   確認する。もう一度打つと、**別の URL** が刷られることを確認する。
3. `sshc engine` をもう一度打ち、**二台目が上がらず**、入口の取り方が出ることを
   確認する。
4. `DISPLAY` も `WAYLAND_DISPLAY` も無い状態で `sshc` を打ち、**URL は刷られ、
   ブラウザは開かない**ことを確認する（エラーにはならない）。
5. エンジンを Ctrl-C で止め、`sshc` が「動いていない」と起こし方を出すことを
   確認する。開いていたコンソールと解錠済みの保管庫が一緒に消えることも確認する。
6. `tmux new -d -s sshc 'sshc engine'` で起こし、端末を閉じてもエンジンが残ること、
   `tmux kill-session -t sshc` で消えることを確認する。

## M5. 実 `~/.ssh` での読み取り専用リハーサル

1. 本番の `~/.ssh` をコピーした使い捨て `HOME` で起動する。
2. Connections、Config、Groups、Keys、Known Hosts、Remote Keys、Diagnostics、History の各画面を開くだけで、何も保存しない。
3. 終了後、`diff -r ~/.ssh "$work/.ssh"` が `sshc/` 配下以外で差分を出さないことを確認する。
4. 期待: 読み取りだけで既存ファイルが 1 バイトも変わらない。

## M6. Android 実機

エミュレータでも代わりになりますが、**PTY と名前解決は実機で一度確かめてください** — どちらもホスト上のテストでは決して分からないものです。

準備:

```sh
go install golang.org/x/mobile/cmd/gobind@latest   # 一度だけ
make android-bind
cd android && ./gradlew assembleDebug
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

1. **起動して logcat の測定値を読む。** `adb logcat -s sshc` に 2 行出る。
   - `probe dns: OK: …` — 名前が引ける。**`FAIL` なら Android で SSH できない**ので、設計に戻る。Android には `/etc/resolv.conf` が無く、cgo リゾルバが netd へ届いているかどうかがここで決まる。
   - `probe shell: OK: /system/bin/sh` — 開けるログインシェルがある。
2. **WebView に UI が出ることを確認する。** 出れば、埋め込み UI と CSP が Android WebView の下で成立している。
3. **保管庫を作り、接続を 1 つ足して、ターミナルを開く。** WebSocket が CSP `connect-src 'self'` を通ることと、リモートセッションが繋がることを同時に見ている。
4. **ローカルシェルを開く。** `/system/bin/sh` が開き、`ls` がアプリの私有ディレクトリを見せる。`creack/pty` が Android の `/dev/ptmx` で動くことの確認である。**ここだけが倒れた場合、リモートセッションには影響しない。**
5. **ホーム画面へ抜けて 30 秒待ち、戻る。** セッションが生きたままであることを確認する。**別アプリに切り替えたら切れるのは、起きるかもしれない話ではなく最初に起きる話である。**
6. **画面を回す。** `configChanges` で Activity を作り直さないので、セッションも WebView の状態も残る。
7. **ハードウェア鍵とエージェント登録が画面に出ないことを確認する。** Keys 画面に `ecdsa-sk` / `ed25519-sk` の項目が無く、「エージェントに追加」が無効になっている。道具が無いことが機能の不在として現れている。
8. **CLI がどこからも案内されないことを確認する。** 打てる端末がないので、`sshc` のコマンドを提示する文言が画面に出てはならない。

## 記録

未実施は空欄のままにせず「未実施」と書きます。空欄は「実施したが記録し忘れた」と区別がつきません。

| 日付 | 項目 | OS | OpenSSH | 結果 | 備考 |
|---|---|---|---|---|---|
| — | M1 | — | — | 未実施 | 検証用リモートホストが必要 |
| — | M2 | — | — | 未実施 | 削除してよいリモートアカウントが必要 |
| — | M3 | — | — | 未実施 | 本番 `~/.ssh` の退避が必要 |
| — | M4 | — | — | 未実施 | 本番 `~/.ssh` の退避が必要 |
| — | M5 | — | — | 未実施 | 本番 `~/.ssh` のコピーが必要 |
| — | M6 | — | — | 未実施 | Android 実機と adb が必要 |

## Android をエミュレータで動かして、中を覗く

**実機が無くても、実機と同じ Chromium の中身が読める。** WebView を Chrome
DevTools に開けば、押した場所にどの要素があるか、どのイベントが発火して何が
打ち消されたか、計算後のスタイルがどうなっているかを、すべて測れる。
`pointer-events` も `contextmenu` も、ここから測って初めて分かった。

```sh
A=~/Library/Android/sdk
$A/emulator/emulator -avd Medium_Phone_API_36.1 -no-snapshot-load &
$A/platform-tools/adb wait-for-device

make android-bind
cd android && ./gradlew clean assembleDebug && cd ..
$A/platform-tools/adb install -r android/app/build/outputs/apk/debug/app-debug.apk
$A/platform-tools/adb shell am start -n com.github.aida0710.sshc/.MainActivity
```

画面を見るのは `adb exec-out screencap -p > shot.png`、触るのは
`adb shell input tap X Y` と `adb shell input swipe X1 Y1 X2 Y2 <ミリ秒>`。
**同じ座標で 900ms の swipe が長押しである。**

中を覗くには、debuggable なビルドが開けている devtools のソケットへ繋ぐ。

```sh
PID=$($A/platform-tools/adb shell cat /proc/net/unix | grep -o "webview_devtools_remote_[0-9]*" | head -1)
$A/platform-tools/adb forward tcp:9222 localabstract:$PID
curl -s http://127.0.0.1:9222/json      # page の webSocketDebuggerUrl が出る
```

あとは CDP の `Runtime.evaluate` を投げるだけで、ページの中で任意の JS が走る。
Node 22 には WebSocket が入っているので、依存は要らない。**Playwright の
connectOverCDP は使えない** ——Android の WebView は browser context 管理を
持たないので、接続の時点で断られる。

デバイスの座標と CSS の座標は次で対応が取れる。倍率は端末ごとに違うので、
決め打ちにせず毎回測ること。

```js
document.addEventListener("touchstart", (e) => {
  console.log(e.touches[0].clientX, e.touches[0].clientY);
}, true);
```

**release ビルドでは devtools は開かない。** `MainActivity` が
`FLAG_DEBUGGABLE` を見てから有効にする——開けば、画面の中身も session cookie も
同じ機械から読める。
