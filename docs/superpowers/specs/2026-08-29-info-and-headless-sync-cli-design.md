# `sshc info` と headless sync CLI

## 目的

画面を持たない Linux でも、次の二つを CLI だけで行えるようにする。

1. SSH 接続を開始せず、`sshc` が実際に使う接続先設定を確認する。
2. 起動済み engine が所有する同期機能を、Web UI と同じ安全性で設定・実行する。

CLI に第 2 の SSH resolver、S3 client、同期アルゴリズム、vault 実装を作らない。
接続と Web UI がすでに使っている実装を owner とし、CLI は入力、engine API の呼び出し、
出力だけを受け持つ。

`connect` サブコマンドは追加しない。既存の役割分担を保つ。

```text
sshc ssh                         人が TUI から選ぶ
sshc ssh <alias>                 明示した相手へ対話接続する
sshc ssh --list                  alias を列挙する
sshc ssh <alias> --non-interactive -- <command>
                                 非対話 SSH を実行する
```

## CLI 契約

追加する呼び出しは次の通りである。

```text
sshc info <alias> [--json]

sshc sync [--json]
sshc sync setup
sshc sync push [--force] [--json]
sshc sync pull [--force] [--json]
sshc sync now [--json]
sshc sync auto on|off [--json]
```

`--json` は指定された操作の最終結果を 1 個の JSON object として stdout に出す。
診断や進捗を stdout に混ぜない。`setup` は秘密を対話入力する wizard なので JSON mode を
持たない。

終了コードは全コマンドで揃える。

| code | 意味 |
|---|---|
| `0` | 操作が完了した |
| `1` | engine 停止、vault lock、通信失敗、同期競合、安全上の拒否など実行時の失敗 |
| `2` | 不正な引数または利用法 |
| `130` | `Ctrl-C` による中断 |

## `sshc info <alias>`

### 値の owner

`info` は engine を必要としない。ローカルの `~/.ssh/config`、到達可能な `Include`、
sshc metadata を読み、現在の `sshc ssh <alias>` と同じ application service と
`effective.Resolve` を通す。その結果を `sshclient.NewTarget` で接続対象へ組み立てる。

表示専用の `effective.Project` は使わない。`Project` は provenance のための投影であり、
`Match` を適用した接続値の owner ではない。

接続用の組み立てを次の三者で共有する。

- `CLIConnection.Open`
- `CLIConnection.Run`
- 新しい read-only の target description

これにより `Include`、`Match`、既定の `Port 22`、token 展開、`ProxyJump`、
`IdentityFile`、接続 encoding の解釈が接続時とずれない。`info` は network connection、
host-key scan、vault read を行わない。

### 出力

人向け表示と JSON は、同じ allowlist DTO から生成する。少なくとも次を含める。

- schema version（JSON のみ）
- alias
- destination の HostName、User、Port
- 展開済み IdentityFile と IdentitiesOnly
- ProxyJump の順序と各 hop の destination
- ProxyCommand が設定されているか
- encoding
- authentication method の試行順
- RequestTTY、StrictHostKeyChecking
- connect timeout、server-alive interval/count
- agent forwarding の有無
- 実装が認識した notice

次は出さない。

- 保存済み password、key passphrase、vault/sync key
- authentication binding digest
- `SetEnv` の値
- `ProxyCommand` のコマンド文字列

最後の二つはユーザーが任意の秘密を書けるためである。存在や変数名を知らせる必要が
あっても値は出さない。汎用的な `effective.Values` 全件 dump は設けない。

JSON の最上位は次の形で固定する。

```json
{
  "schemaVersion": 1,
  "alias": "edge",
  "destination": {"hostName": "192.0.2.10", "user": "manager", "port": "22"},
  "identityFiles": [],
  "identitiesOnly": false,
  "proxyJump": [],
  "proxyCommandConfigured": false,
  "encoding": "utf-8",
  "authenticationMethods": ["publickey", "keyboard-interactive", "password"],
  "requestTTY": "",
  "strictHostKeyChecking": "",
  "connectTimeoutSeconds": 0,
  "serverAliveIntervalSeconds": 0,
  "serverAliveCountMax": 3,
  "agentForward": false,
  "notices": []
}
```

JSON の配列は空なら `[]` とし、型を変えない。解決不能な設定は部分的な値を成功として
出さず、終了コード 1 とする。

## sync CLI と engine の境界

### engine を必須にする理由

sync の owner は起動済み engine である。CLI は handoff file を読み、互換性のある
engine にだけ接続する。sync status を含むすべての sync command は vault の解錠を
前提とする。vault が無い、または lock 中なら API session を作らず拒否し、既存の
`sshc vault create|unlock` を案内する。

CLI が sync 設定ファイルを直接復号したり、S3 へ直接接続したりはしない。そうすると、
Web と CLI で以下が分岐するためである。

- setup の到達確認と保存順序
- snapshot encryption key
- CAS/ETag と remote revision
- pull transaction と conflict policy
- auto sync の現在状態
- action token による force-push 認可

### Web API をそのまま呼ぶための認証橋

現在の `/api/v1/*` は browser session cookie、port-origin scoped CSRF token、
`Origin`、`Sec-Fetch-Site` を要求する。一方、既存 CLI route は 0600 の handoff file に
ある `X-SSHC-CLI` secret で認証する。同じ sync handler を `/cli` 以下へ複製しない。

代わりに handoff secret でだけ呼べる `POST /cli/session` と
`DELETE /cli/session` を一組設ける。

1. endpoint は command-scoped の通常 API session と CSRF token を発行する。
2. session cookie は既存 Web session と同じ属性で返す。
3. CLI は exact engine origin、`Sec-Fetch-Site: same-origin`、CSRF header を付け、
   既存 `/api/v1/sync` と `/api/v1/actions` を呼ぶ。
4. CLI は終了時に session を best effort で revoke する。
5. 中断や kill で revoke できない場合に備え、CLI session は短い hard TTL を持つ。

TTL 切れを mutation の自動再試行理由にはしない。変更要求の応答を受け取れなかった場合、
CLI は結果不明として status の再確認を案内する。setup の途中で TTL が切れた場合も、
check 済みの値を別 session から黙って保存せず、setup 全体をやり直す。

Web 用 bootstrap token を発行する既存 `/cli/open` は流用しない。流用すると CLI を
一度実行しただけで、まだ browser が消費していない UI URL を失効させる競合が起きる。

handoff secret を読めるプロセスは現状でも `/cli/open` から Web session を作れるため、
この橋は authority を拡張しない。ただし API 全体を呼べる credential を disk、argv、
environment、log へ出さず、memory 内だけで command lifetime に限定する。

### 共通 client

`cmd/sshc` に transport/authentication だけを所有する小さい engine client を置く。
責務は次に限定する。

- 検証済み handoff の読取
- CLI session の取得と revoke
- exact origin、cookie、CSRF、Fetch Metadata header
- redirect 拒否
- request timeout と bounded response body
- `application/problem+json` の `code` を型付き error へ変換
- secret-bearing request body の one-shot 送信と可能な範囲での zeroing

`status`、`vault`、`sync` に共通化できる handoff/HTTP primitives はこの client へ寄せる。
ただし vault password の zeroable JSON encoder と不確実な変更結果の扱いは弱めない。
汎用的な「任意 path を呼べる public helper」にはせず、呼べる operation は型付き method
として列挙する。

通常の request/response は `internal/api` の OpenAPI generated types を使う。
setup の access key、secret access key、sync key は immutable Go string の複製を増やさない
ため、既存 vault CLI と同じ one-shot/zeroable payload builder を使って同じ wire schema を
生成する。この例外は OpenAPI contract test で wire shape を固定する。

## `sshc sync`

`sshc sync` は `GET /api/v1/sync` の状態を人向け表で出す。`--json` は次の envelope で
同じ `api.SyncStatus` を返す。

```json
{"schemaVersion":1,"success":true,"status":{}}
```

人向け表示は secret を含まない次の項目を持つ。

- configured / vault locked / sync key configured
- endpoint、bucket、path、region、direction
- auto enabled、phase、直近の時刻と detail
- last synced time、origin、file count
- last operation の kind、file/object count、source/snapshot/upload/download bytes、
  written/removed count、completed time

access key ID、secret access key、sync key は status API に存在せず、CLI にも出ない。

## `sshc sync setup`

setup は stdin と prompt/秘密を表示する controlling terminal が利用できる場合だけ動く。
pipe、argv、environment から credential を受け取る option は設けない。通常の成功概要は
stdout に出してよいが、生成 sync key は stdout ではなく prompt と同じ terminal に出す。

wizard は次を順に尋ねる。

1. HTTPS endpoint
2. bucket
3. path（空可）
4. region（既定 `auto`）
5. direction（`both` / `push` / `pull`、既定 `both`）
6. access key ID（no echo）
7. secret access key（no echo）

その後 `POST /api/v1/sync/setup/check` を呼ぶ。

- target が `incomplete` なら保存せず拒否する。
- target が `existing` なら sync key を no-echo で尋ねる。
- target が `empty` なら engine に sync key を生成させる。

`PUT /api/v1/sync/setup` には check で得た state、history presence、ETag をそのまま渡す。
check 後に remote が変われば `sync_setup_target_changed` で拒否し、保存しない。credential と
sync key の暗号化保存は、engine が remote/network/cryptographic check をすべて完了した後
にだけ行う。

生成された sync key は対話 terminal に一度だけ表示し、再表示 API や JSON output には
含めない。CLI は入力 buffer と request body を操作終了後に zeroing する。

## push

### `sshc sync push`

1. `GET /api/v1/sync/push` で engine が作った draft と commit message を取得する。
2. `POST /api/v1/sync/push` を呼ぶ。
3. engine の通常 CAS が remote change を検出したら拒否する。

CLI は snapshot を作らず、message や差分を独自計算しない。

### `sshc sync push --force`

`--force` 自体を明示確認とし、追加の yes/no prompt は出さない。ただし無条件 overwrite は
しない。

1. `POST /api/v1/actions` で `sync.force_push` の action token を取得する。
2. token は engine がその時点で検査した exact remote ETag の evidence に結び付く。
3. `POST /api/v1/sync/force-push` に一度だけ提示する。
4. 検査後に remote が変われば token/evidence または conditional write が拒否し、
   CLI は自動的に新しい対象へ force し直さない。

再試行はユーザーがコマンドをもう一度実行したときだけ行う。

## pull

pull は engine の preview/apply 二段階 API を CLI が連続して呼ぶ。preview の remote ETag と
revision を apply に必ず渡す。

### `sshc sync pull`

- conflict が一件でもあれば適用しない。
- removal が一件でもあれば適用しない。
- 安全な write/add だけなら exact preview を apply する。
- preview 後に remote が変われば `preview_stale` または `sync_remote_moved` で拒否する。

### `sshc sync pull --force`

`--force` 自体を remote-authoritative resolution の確認とし、追加 prompt は出さない。

- conflict は remote を採用する。
- remote 側の removal を適用する。
- apply には preview で得た exact ETag/revision と `resolve=remote` を渡す。
- preview 後に remote が変われば拒否し、新しい内容を自動確認したことにはしない。

`--force` は競合方針を変えるだけで、path safety、snapshot schema、envelope、transaction、
cost limit の検査を無効にしない。

## `now` と `auto`

```text
sshc sync now
sshc sync auto on
sshc sync auto off
```

既存 `POST /api/v1/sync/now` と `PUT /api/v1/sync/auto` を呼ぶ。CLI が独自 timer や daemon
を持たない。`auto on` の永続化、即時巡回、phase/status は engine の既存実装に従う。

## JSON result と error

`--json` を持つ mutation の成功は共通 envelope に operation 固有の generated API response
を入れる。

```json
{"schemaVersion":1,"success":true,"result":{}}
```

失敗時は stdout に一つだけ次を出し、stderr へ説明を重複させない。

```json
{
  "schemaVersion": 1,
  "success": false,
  "failure": {"kind": "sync_remote_moved", "retryable": true}
}
```

`kind` は engine の problem `code` または CLI 固有の安定した分類である。engine の固定文言
`message` や raw network error は JSON contract にしない。人向け mode は回復手順を stderr
に書く。

secret、cookie、CSRF token、action token、handoff secret、request body は、成功・失敗・
debug のいずれでも出力しない。

## テスト

### parser と出力

- `info` と全 sync invocation の受理・拒否 matrix
- flag 順序、重複 flag、未知 flag、欠けた引数が exit 2
- human stdout/stderr と JSON one-object contract
- JSON array/null、schema version、failure kind の固定
- あらゆる応答とエラーに secret が混ざらないこと

### info

- `Include` と `Match` を含む設定が接続と同じ値になること
- `HostName`、`User`、既定 `Port 22`、IdentityFile、ProxyJump、encoding
- `effective.Project` の値を誤って使わない acceptance test
- resolver refusal で部分結果を出さないこと
- SetEnv と ProxyCommand 本文を出さないこと
- info が engine、vault、network を必要としないこと

### engine client と認証

- handoff owner/version/protocol 不一致の拒否
- redirect 拒否、response size limit、timeout、cancellation
- CLI session が exact origin、cookie、CSRF、Fetch Metadata を満たすこと
- CLI session の revoke と hard TTL
- CLI session 発行が未使用 Web bootstrap URL を失効させないこと
- problem+json code の型付き error 変換

### sync

- status/setup/push/pull/now/auto が既存 `/api/v1` route を呼ぶこと
- setup check 失敗、`incomplete`、target change で secret settings を保存しないこと
- setup の secret input が TTY 必須かつ no-echo であること
- 通常 push の CAS refusal
- force push action token が exact ETag に束縛され、remote race を拒否すること
- 通常 pull が conflict/removal を適用しないこと
- force pull が remote resolution と removal を exact preview にだけ適用すること
- preview/apply 間の remote change を拒否すること
- vault lock、sync key missing、wrong key、unsupported snapshot、cost refusal の表示と exit code

### 回帰

- Web sync test は同じ handler/service を通ったまま変更しない
- `go test ./...`
- race detector を含む対象 package test
- `go vet ./...`
- repository が定める dead-code/check script
- macOS/Linux/Windows cross build
- 専用 test target が利用できる場合、実機の headless engine を使う
  setup/status/push/pull/force race の確認

実機確認では既存の remote を無断で上書きしない。専用の空 target を使い、force の race
確認も別 target で行う。

## スコープ外

- `sshc connect` の復活
- engine を CLI が暗黙起動すること
- sync credential の argv/environment/file import
- engine なしで S3 へ接続する fallback
- 無条件 PUT や ETag を無視する force
- sync history/diff/key rotation の新しい CLI
- raw effective configuration の全件 dump

## 批判と採用判断

「できる限り共通実装を呼ぶ」は正しいが、DRY 自体を目的にすると認証と表示まで巨大な
抽象へ押し込める危険がある。本設計では owner を次のように限定する。

- SSH の意味は application/effective/sshclient が所有する。
- sync の意味と mutation safety は engine handler/service が所有する。
- engine HTTP の認証・制限・problem decode は CLI engine client が所有する。
- human/JSON presentation は各 command の DTO/renderer が所有する。

したがって human UI と JSON renderer は無理に一つの汎用 formatter にせず、同じ DTO を
別々に表示する。`info` を engine API に寄せないのも意図的である。engine が止まった状況で
SSH 設定を診断できる価値が、HTTP client の見かけ上の共通化より大きい。
