# 手動受け入れ試験

自動テストの対象外とする操作をまとめます。実リモート接続、実 `authorized_keys` の変更、実 ssh-agent への登録、Android 実機での通知からの停止と WebView の動作は手動で確認します。

実施前に必ず読むこと。

- 実施は使い捨ての `HOME` で行い、本番の `~/.ssh` では行いません。
  `work="$(mktemp -d)"; cp -R ~/.ssh "$work/.ssh"; HOME="$work" ./bin/sshc`
- `./bin/sshc` はブラウザを開きません。アクセス URL を標準出力に 1 行表示するため、手動で開いてください。
- 本番の `~/.ssh` を使う項目（ssh-agent とデスクトップ）は、事前に `~/.ssh` と `~/.ssh/sshc` を別ディレクトリへ退避してから行います。
- 各項目に日付、OS と OpenSSH のバージョン、結果を記録します。M3 は macOS でも Linux でも実施できます。

## M1. 実リモートホストへの接続テスト

1. 使い捨て `HOME` に、自分が管理する検証用ホストの `Host` ブロックを作る。
2. Diagnostics タブで「到達性」を実行し、`ProxyJump は使用していない` という注記が出ることを確認する。
3. 「認証テスト」を実行する。
4. 期待結果: 認証成功と、成功した認証方式（`publickey` など）が表示される。説明は 8 KiB 以内に切り詰められ、鍵本文とパスフレーズは表示されない。
5. 実行可能ディレクティブを持つ設定では、確認ダイアログに実際のコマンド文字列が表示され、確認するまで開始しないことを確認する。
6. Known Hosts パネルで「Scan」を実行し、候補が「unverified」と表示されること、別経路で得た fingerprint を入力するか明示的に承認するまで「Add to known_hosts」が押せないことを確認する。この操作は実際に対象アドレスへ接続するため、自動テストには含めていません。
7. 接続を開いて実際に操作する。`vim` を開いてウィンドウを広げ、`window-change` によって描画が追随することを確認する。`LocalForward` を設定して接続し、転送が一覧に表示されること、そのポート経由で接続先に到達できること、コンソールを閉じるとポートが解放されることを確認する。
8. 未知のホストへ接続し、fingerprint を含む確認プロンプトが端末に表示されることを確認する。`yes` では接続されて `known_hosts` に 1 行追加され、`no` では接続されずファイルも変更されないことを確認する。次に、既知のホストの鍵を変更して接続し、確認プロンプトを表示せず接続を拒否することを確認する。
9. remote shellを`exit`で終了し、終了表示内の「再接続」を押す。同じpaneで新しいshellが開き、終了前のscrollbackが境界メッセージより上に残ることを確認する。再び終了させ、再接続を押した直後にconsoleを閉じた場合は、接続完了後にsessionが復活しないことを確認する。

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
5. 登録中に `ps -ax | grep ssh-add` を実行し、`ssh-add` プロセスが起動しないことを確認する。このアプリケーションは agent プロトコルを直接使用し、プロセス内で鍵を復号してから agent に渡す。

自動テストは `x/crypto/ssh/agent` のキーリングを相手に本物のプロトコルで往復します（`internal/keys/agent_test.go`）。実際のユーザーの agent に対して成立することを示せるのはこの手動試験だけです。

## M4. エンジンとアクセス URL

アクセス URL の発行、2 個目のエンジンの拒否、`.xterm` の描画は E2E テストで確認します。
この項目では、`xdg-open` / `open` が起動するブラウザ、ブラウザによる loopback origin の扱い、GUI のない環境でブラウザを起動しないことを実環境で確認します。

1. 端末で `sshc engine` を起動し、アクセス URL が表示されず、次に実行できる `sshc vault ...` の案内だけが表示されることを確認する。
2. 別の端末で `sshc` を実行し、URL が表示されて既定のブラウザで開かれることを確認する。再度実行し、前回とは異なる URL が表示されることを確認する。
3. `sshc engine` を再度実行し、2 個目のエンジンが起動せず、アクセス URL の取得方法が表示されることを確認する。
4. `DISPLAY` と `WAYLAND_DISPLAY` がない状態で `sshc` を実行し、エラーにならず URL だけが表示され、ブラウザは起動しないことを確認する。
5. Ctrl-C でエンジンを停止し、`sshc` がエンジン停止中であることと起動方法を表示することを確認する。開いていたコンソールと解錠済みの保管庫も終了することを確認する。
6. `tmux new -d -s sshc 'sshc engine'` で起動し、端末を閉じてもエンジンが動作し続け、`tmux kill-session -t sshc` で終了することを確認する。

## M5. 実 `~/.ssh` での読み取り専用リハーサル

1. 本番の `~/.ssh` をコピーした使い捨て `HOME` で起動する。
2. Connections、Config、Groups、Keys、Known Hosts、Remote Keys、Diagnostics、History の各画面を開くだけで、何も保存しない。
3. 終了後、`diff -r ~/.ssh "$work/.ssh"` が `sshc/` 配下以外で差分を出さないことを確認する。
4. 期待: 読み取りだけで既存ファイルが 1 バイトも変わらない。

## M6. Android 実機

エミュレータでも確認できますが、PTY と名前解決は Android 実機でも少なくとも一度確認してください。ホスト上のテストだけでは検証できません。

準備:

```sh
go install golang.org/x/mobile/cmd/gobind@latest   # 一度だけ
make android-bind
cd android && ./gradlew assembleDebug
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

1. アプリを起動し、`adb logcat -s sshc` に出る 2 行を確認する。
   - `probe dns: OK: …`: 名前解決に成功している。`FAIL` の場合、Android では SSH 接続できない。Android には `/etc/resolv.conf` がないため、cgo リゾルバが netd を利用できることを確認する。
   - `probe shell: OK: /system/bin/sh`: 使用可能なログインシェルがある。
2. WebView に UI が表示されることを確認する。これにより、埋め込み UI と CSP が Android WebView で機能することを確認できる。
3. 保管庫を作成し、接続を 1 件追加してターミナルを開く。WebSocket が CSP の `connect-src 'self'` を満たし、リモートセッションに接続できることを確認する。
4. ローカルシェルを開く。`/system/bin/sh` が起動し、`ls` でアプリ専用ディレクトリが表示されることを確認する。これは `creack/pty` が Android の `/dev/ptmx` で動作することの確認であり、失敗してもリモートセッションには影響しない。
5. ホーム画面に移動して 30 秒待ち、アプリに戻った後もセッションが維持されていることを確認する。別アプリへの切り替え時にセッションが終了することも確認対象とする。
6. 画面を回転し、`configChanges` によって Activity が再作成されず、セッションと WebView の状態が維持されることを確認する。
7. Keys 画面に `ecdsa-sk` / `ed25519-sk` が表示されず、「エージェントに追加」が無効であることを確認する。Android で利用できない機能を UI に表示しないことの確認である。
8. UI のどこにも CLI の利用を案内する文言が表示されないことを確認する。Android にはコマンドを入力できる端末がないためである。
9. Android 13以降で初回起動、強制停止後の起動、taskから除去してすぐ再起動を試し、毎回Web UIへ到達することを確認する。Service再生成などで同一processの起動要求が重複しても、古いengineを置き換えて通常画面へ収束し、二重起動エラー画面を表示しないことをlogcatでも確認する。private storageを開けない場合はstorage errorになることも確認する。

## M7. 実ホストでのSFTP

OpenSSHコンテナに対するプロトコル往復は`make integration`で自動検査します。この項目では実際の権限、遅延、サーバー固有設定とブラウザ操作を確認します。

1. 削除してよい検証用directoryを実ホストに作り、SFTPから開く。
2. nested directory、空directory、小さいtext／binaryを含むフォルダをDrag & Dropし、階層と空directoryが維持され、進捗とfile別結果が表示されることを確認する。
3. 別のフォルダをアップロードし、既存名へ衝突させる。上書き確認で1件を上書きし、別の1件をskipして、残りが続行されることを確認する。別の転送では取消し、未開始fileが送られないことを確認する。
4. textをMonacoで開き、別のSSH sessionから内容を変更した後に保存して、競合として拒否されることを確認する。
5. file download、directoryのZIP download、rename、空directoryの作成と削除を確認する。非空directoryが再帰削除されないことも確認する。
6. upload／file download／folder downloadを3件以上追加し、Transfer Managerが同時2件だけを実行すること、fileごとにbytes、速度、残り時間、attempt、statusを表示することを確認する。
7. 2 MiBを超えるfileをuploadし、転送中にpauseしてからresumeする。別画面へ移動して戻っても同じjobとbytes進捗が残り、完了前はtarget名のfileが見えず、完了後だけ一覧へ現れ、別画面でも完了通知が出ることを確認する。
8. folder uploadのうち1 fileだけを失敗させ、batchの「失敗のみ再試行」で成功済みfileを再送せず失敗fileだけattemptが増えることを確認する。
9. 転送中にnetworkを一時的に切り、uploadがremote part sizeから、file downloadがHTTP Rangeから再開することを確認する。folder ZIPはretry時に先頭からやり直すことも確認する。
10. pause／resume／retry／cancelをuploadとfile downloadで試し、cancelしたuploadのpart fileが削除されること、失敗通知を閉じられることを確認する。
8. fileとdirectoryの権限をchmodで変更し、一覧の権限・更新日時・種別列とsortを確認する。symlinkにはchmodが表示されないことを確認する。
9. 作成したfileとdirectoryを削除して原状復帰する。

## M8. Workspace Command Center

1. 同じaliasを2つ含み、別aliasを1つ含む3 paneのWorkspaceを開く。
2. 各paneの対話shellで別々のdirectoryへ移動し、直接入力した`pwd`のpreviewが同じaliasを含む3 sessionすべてを表示することを確認する。
3. 送信後、別SSH接続の結果欄ではなく各paneにそのpane自身のcwdが表示されることを確認する。
4. 1 paneを再接続中にして開き直し、未接続paneが送信対象外と表示され、aliasを使った代替接続が作られないことを確認する。
5. preview後に対象paneを再接続してから送信し、preview変更として拒否され、新しいshellへ古いcommandが届かないことを確認する。
6. 通常変数を持つSnippetは展開後commandをpreviewして送信でき、secret変数を持つSnippetはterminal表示とshell履歴への漏洩防止のため送信できないことを確認する。
7. `cat`などがforegroundで標準入力を待つpaneへ送るとshell commandとしてではなくそのprocessの入力になることを確認し、modalの注意書きと一致することを確認する。

## M9. Workspace paneの配置交換

1. 3 pane以上のnested Workspaceを開き、各paneで識別できるcommand outputを表示する。
2. 1つ目のpaneの移動handleを別paneへDrag & Dropし、2つのpane位置だけが交換されることを確認する。
3. 交換後も各terminalのscrollbackと接続が対応するpaneに残り、split方向と比率が変わらないことを確認する。
4. 移動handleを2つ順に選択し、Drag & Dropと同じ交換が行われることを確認する。最初のhandleでEscapeを押すと選択を解除できることも確認する。
5. Workspaceを保存して再度開き、交換後の配置でaliasごとに新規接続されることを確認する。
6. ローカルシェルを2つ開き、一方の一覧行をもう一方の上下左右へdropして分割できることを確認する。保存後に再度開くと、同じ配置で新しいローカルシェルが2つ開始されることを確認する。
6. 縦横それぞれのseparatorをDragして比率を変更し、保存・再オープン後に復元されることを確認する。矢印キーでも5%ずつ変更できることを確認する。
7. 任意paneをFocus Modeへ切り替え、他paneの接続を閉じずに単一paneだけが表示されること、Escとtoolbarの両方で元layoutへ戻ることを確認する。

## M10. Terminal検索・履歴・補完

1. 長い出力を作り、Ctrl/Cmd+Fでscrollback内の語を検索する。Enter／Shift+Enterと前後buttonで全一致を巡回できることを確認する。
2. 同じprefixを持つcommandを複数回実行し、入力中に頻度順の候補が表示され、Tabまたは候補buttonで残りが入力されることを確認する。
3. SSH terminalでabsolute remote pathを入力し、SFTPで読める親directoryの候補が表示されることを確認する。relative pathではcwdを推測した候補が出ないことを確認する。
4. terminalを閉じて開き直し、以前のcommand履歴がdiskやWorkspaceから復元されないことを確認する。

## 記録

未実施は空欄のままにせず「未実施」と書きます。空欄は「実施したが記録し忘れた」と区別がつきません。

| 日付 | 項目 | OS | OpenSSH | 結果 | 備考 |
|---|---|---|---|---|---|
| 未記録 | M1 | 未記録 | 未記録 | 未実施 | 検証用リモートホストが必要 |
| 未記録 | M2 | 未記録 | 未記録 | 未実施 | 削除してよいリモートアカウントが必要 |
| 未記録 | M3 | 未記録 | 未記録 | 未実施 | 本番 `~/.ssh` の退避が必要 |
| 未記録 | M4 | 未記録 | 未記録 | 未実施 | 本番 `~/.ssh` の退避が必要 |
| 未記録 | M5 | 未記録 | 未記録 | 未実施 | 本番 `~/.ssh` のコピーが必要 |
| 未記録 | M6 | 未記録 | 未記録 | 未実施 | Android 実機と adb が必要 |
| 未記録 | M7 | 未記録 | 未記録 | 未実施 | 削除してよい実ホストのdirectoryが必要 |
| 未記録 | M8 | 未記録 | 未記録 | 未実施 | 複数paneで接続できる検証用ホストが必要 |
| 未記録 | M9 | 未記録 | 未記録 | 未実施 | 複数paneで接続できる検証用ホストが必要 |
| 未記録 | M10 | 未記録 | 未記録 | 未実施 | 長い出力とSFTP可能な検証用ホストが必要 |

## Android エミュレータで WebView を調査する

実機がなくても、エミュレータの WebView を Chrome DevTools で調査できます。
対象要素、発生したイベント、キャンセルされたイベント、計算後のスタイルを確認できます。
`pointer-events` と `contextmenu` の問題もこの方法で特定しました。

```sh
A=~/Library/Android/sdk
$A/emulator/emulator -avd Medium_Phone_API_36.1 -no-snapshot-load &
$A/platform-tools/adb wait-for-device

make android-bind
cd android && ./gradlew clean assembleDebug && cd ..
$A/platform-tools/adb install -r android/app/build/outputs/apk/debug/app-debug.apk
$A/platform-tools/adb shell am start -n com.github.aida0710.sshc/.MainActivity
```

画面の取得には `adb exec-out screencap -p > shot.png`、操作には
`adb shell input tap X Y` と `adb shell input swipe X1 Y1 X2 Y2 <ミリ秒>`。
同じ座標を始点と終点に指定した 900 ms の swipe は長押しとして扱われます。

内部状態を調査するには、debuggable ビルドが開く DevTools ソケットへ接続します。

```sh
PID=$($A/platform-tools/adb shell cat /proc/net/unix | grep -o "webview_devtools_remote_[0-9]*" | head -1)
$A/platform-tools/adb forward tcp:9222 localabstract:$PID
curl -s http://127.0.0.1:9222/json      # page の webSocketDebuggerUrl が出る
```

CDP の `Runtime.evaluate` を使用すると、ページ内で任意の JavaScript を実行できます。
Node 22 は WebSocket を内蔵しているため追加依存は不要です。Playwright の
`connectOverCDP` は、browser context 管理を持たない Android WebView には接続できません。

デバイスの座標と CSS の座標は次で対応が取れる。倍率は端末ごとに違うので、
決め打ちにせず毎回測ること。

```js
document.addEventListener("touchstart", (e) => {
  console.log(e.touches[0].clientX, e.touches[0].clientY);
}, true);
```

リリースビルドでは DevTools を有効にしません。`MainActivity` は
`FLAG_DEBUGGABLE` を確認してから有効にします。DevTools が有効な場合、同じ端末から画面の内容と session cookie を読み取れるためです。
