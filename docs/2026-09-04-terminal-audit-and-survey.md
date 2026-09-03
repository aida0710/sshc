# sshcターミナル監査・競合サーベイ — 2026-09-04

## 結論

最優先は、新しいプロトコルを増やすことではなく、端末入力と再接続の境界を安全にすることだった。この監査では、改行・制御文字を含む貼り付けを送信前に確認するSafe Pasteと、WebSocket再接続時に未受信分だけを再生するbyte cursorを実装した。

次に実装する価値が高いのはOSC 133によるcommand／output zoneと、長時間commandの完了通知である。Moshは価値がある一方、UDP、remote binary、端末状態同期、Windows対応、license境界を同時に扱うため、先に小さい改善を積む。

## 調査範囲

比較には各製品の公式文書または公式repositoryだけを使用した。

| 製品 | 確認した強み | sshcへの判断 |
|---|---|---|
| iTerm2 | 複数行貼り付けと末尾改行付き1行の警告、command単位の選択、tmux integration | Safe Pasteを先行実装。command zoneと永続sessionは後続 |
| SecureCRT | 編集可能な複数行paste確認、command window、session snapshot、keyword highlight、log rotation | 安全な入力と監視を優先し、無条件のterminal logは導入しない |
| Warp | commandとoutputをBlockとして扱い、長時間commandや入力待ちを通知 | OSC 133を使った構造化と通知を次候補にする |
| WezTerm | OSC 7、OSC 133、OSC 1337によるshell integration | 既存OSC 7実装を拡張する形が最小 |
| Termius | SSH、SFTP、port forwarding、Mosh、vault、同期、session logを統合 | sshcが既に持つ範囲と、Mosh／session logの差を分離して判断 |
| Mosh | 回線変更、断続接続、local echo、UDP上の状態同期 | 高遅延・mobile用途には有効だが独立projectとして扱う |

iTerm2は複数行貼り付けと、末尾が改行の1行貼り付けに対する警告を公式に備える。[iTerm2 Menu Items](https://iterm2.com/documentation-menu-items.html) SecureCRTも入力内容を表示して編集できる複数行paste dialogを持つ。[SecureCRT Features](https://www.vandyke.com/products/beta/securecrt/features.html) この2製品に共通する安全境界はsshcにも直接適用できる。

Warpはcommandとoutputを1つのBlockとして扱い、copy、search、filter、bookmark、navigationの単位にしている。[Warp Blocks](https://docs.warp.dev/terminal/blocks) また、長時間commandの完了と入力待ちをdesktop通知できる。[Warp Notifications](https://docs.warp.dev/terminal/more-features/notifications) WezTermの公式文書ではOSC 133がprompt、input、outputのzoneを定義するために使われる。[WezTerm Shell Integration](https://wezterm.org/shell-integration.html) sshcではshellの履歴を推測せず、この明示信号を使う。

Termiusの現行planにはSSH、SFTP、port forwarding、Mosh、local vaultが含まれ、上位planでは同期とsession logも提供される。[Termius Pricing](https://termius.com/pricing) Mosh自体はnetwork間roaming、断続接続、local echoを目的とし、UDP上の状態同期を使用する。[Mosh](https://mosh.org/) これは通常のSSH stream再接続とは別のtransport設計である。

## 優先順位

| 優先度 | 候補 | 規模 | 判断 |
|---|---|---:|---|
| P0 | Safe Paste | S | 実装済み。改行、末尾Enter、C0／DELを送信前に表示する |
| P0 | byte cursorによる差分再生 | M | 実装済み。再接続ごとの全scrollback重複を止める |
| P0 | OSC 133 command zone＋長時間command通知 | M | 次候補。既存OSC 7 parserとagent通知基盤を再利用する |
| P1 | 実際にnegotiationしたSSH algorithmの表示 | S〜M | `AlgorithmsConnMetadata`から取得し、Diagnosticsへ出す |
| P1 | opt-inのremote tmux session | L | sshc独自daemonを置かず、利用者が選んだtmuxだけを対象にする |
| P2 | passive keyword／regex highlight | M | 出力を変更せず、browser内の表示だけに適用する |
| P2 | 明示的なsession evidence保存 | L | 既定off、保存先・対象・redaction限界を確認してから開始する |
| P3 | Mosh | XL | UDP到達性、remote binary、Windows、license、scrollbackの設計後 |
| P3 | native HTTP CONNECT proxy | L | OpenSSH `ProxyCommand`との役割分担を先に決める |

GoのSSH libraryはnegotiation結果を返す`AlgorithmsConnMetadata`を提供している。[Go `x/crypto/ssh`](https://pkg.go.dev/golang.org/x/crypto/ssh#AlgorithmsConnMetadata) 推測した設定値ではなく、実接続の暗号・鍵交換・host key・MACを表示できるため、Diagnosticsの改善として独立に実装できる。

## 今回実装した安全境界

### Safe Paste

- 改行またはC0／DEL制御文字を含むclipboard textは、PTYへ1 byteも送る前に確認する。
- previewは制御文字を可視化し、4,096 Unicode文字で打ち切る。元の内容はlog、history、永続storageへ書かない。
- 「中止」「末尾のEnterを1つだけ除く」「そのまま貼り付け」を分ける。
- 右clickの非同期clipboard readはterminalを閉じた時点で失効させ、別sessionへ送らない。
- 確認中の内容をsession IDへ束縛し、pane切替後には表示も送信も行わない。

### WebSocket差分再生

- stream ticketへsession IDとabsolute byte cursorを束縛する。
- serverはreplay範囲を同じlockで取得してからlive streamを登録し、境界の出力を取りこぼさない。
- clientは受信byte数をcursorとして次のticket要求へ渡す。UTF-8 decoderは通常の再接続では維持する。
- cursorがring bufferより古い場合だけtruncatedを通知し、保持中の最古byteから再生する。
- cursorがserver末尾より先ならticket発行を拒否し、別sessionのcursorで将来出力を飛ばさない。

### Vault更新をcommit後にだけ公開する

- 9個あった更新経路を1つの`mutateVault`へ統一し、live vaultではなくcloneを変更する。
- unlock時に取得したbaselineをCAS条件に使い、Commitが成功した場合にだけlive vaultを差し替える。
- `Initialise`は「不存在」CASへ直し、同時に作られたvaultを上書きしない。
- 根拠: `internal/secret/service.go`。

### SFTP転送queueの永続化失敗を成功として扱わない

- `persistJobsLocked`のerrorを全呼出元へ伝播し、失敗時はjob、jobOrder、activeJobs、dataPlaneをsnapshotから復元する。
- `ListJobs`と`ClearFinished`はerrorを返すようになり、HTTP層とCLIまで届く。
- 壊れた`transfers.json`はengine起動を止めない。原文を端末内へ退避してから空のqueueで開始する。退避先は同期対象外である。
- Remote→Remoteは、転送先への公開や転送元の削除が起こる前に意図をdiskへ記録する。terminal commitだけが失敗した場合はreattach＋`sftp_reconciliation_required`にし、実行できる操作をcancelだけへ絞る。restart後も自動では再実行しない。
- 根拠: `internal/sftp/jobs.go`, `queue_store.go`, `remote_jobs.go`, `transfer.go`, `internal/httpserver/sftp.go`。

### CLI再帰downloadの上限

- 深さ64、ルート込み10,000項目、合計1,024 MiBを既定上限にし、Webのfolder archiveと揃える。
- `--max-depth`、`--max-entries`、`--max-total-size`で意図的に引き上げる。上限超過は転送開始前に`recursive_limit`で停止する。
- 不正なentry pathと負のsizeを拒否し、計画後にremote fileが増大しても予定size+1 byteより多く書かない。
- 根拠: `cmd/sshc/sftp.go`, `cmd/sshc/invocation.go`。

### TOTPの誤送信防止

- prompt判定を、文字列中のどこかに`otp`を含む部分一致から、先頭文法と実在する説明付き表現だけへ狭める。`Password for otp-admin:`をOTP質問と誤認してワンタイムコードを送らない。
- 根拠: `internal/totp/totp.go`。

### Web側の状態競合

- terminal session一覧はmutation世代で逆順応答を無効化し、stale復元が作ったsessionを直列にcloseする。`busy`は単一booleanから保留操作カウンタへ変更した。
- SFTPは未保存tabを閉じる経路へ確認を通し、古い一覧応答でsessionを復活させない。
- ConnectionTreeは同一Host blockの複数Include投影を物理位置＋投影出現順で一意化し、ARIA description idを`useId()`で分離する。
- 根拠: `web/src/terminal/sessions.ts`, `web/src/features/workspaces/TerminalWorkspace.tsx`, `web/src/sftp/SFTPWorkspace.tsx`, `web/src/connections/ConnectionTree.tsx`。

### Androidの戻る操作

- `ComponentActivity`と`OnBackPressedDispatcher`へ統一し、Android 16のpredictive back lint errorを解消した。追加した`androidx.activity`の依存検証SHA-256も登録した。
- 根拠: `android/app/src/main/java/com/github/aida0710/sshc/MainActivity.java`, `android/app/build.gradle.kts`, `android/gradle/verification-metadata.xml`。

### 方針として入れないもの

sshc独自のcommand historyとautocompleteは再導入しない。shellや接続先applicationの編集・履歴・補完と競合するためである。command単位の操作は、推測した入力ではなくOSC 133の明示信号から作る。

## 完了条件

Safe Pasteはunit testだけでなく、実browserで「確認前のWebSocket inputが0件」「末尾Enterを除く操作が1 frameだけ送る」を確認する。差分再生はserverのring境界、ticketの単回利用、WebSocket metadata、clientの再接続cursorをそれぞれ回帰testで固定する。

## 検証（2026-09-04）

| 対象 | 結果 |
|---|---|
| `go test -count=1 ./...` | 全package成功 |
| `go test -race -count=1 ./...` | 全package成功 |
| `go vet ./...`、`GOOS=android GOARCH=arm64` build、`scripts/ci/deadcode.sh` | 成功 |
| Web unit | 125 files／1,218 tests成功 |
| TypeScript、ESLint、Pages build | 成功 |
| Playwright E2E | 156件成功 |
| `make generate`、`npm run build --prefix web` | 生成物と埋込みUIは2回連続で同一出力 |

Safe Pasteの受入条件は`web/e2e/terminal.spec.ts`の`reviews a multiline paste before sending any terminal input`で固定した。実Chromiumで、確認dialog表示中にWebSocketへ送られた入力frameが0件であること、「末尾のEnterを除いて貼り付け」が1 frameだけを送り末尾のCRを含まないことを検査する。差分再生はserverのring境界、ticketの単回利用、WebSocket metadata、clientの再接続cursorをそれぞれunit／HTTP testで固定した。

