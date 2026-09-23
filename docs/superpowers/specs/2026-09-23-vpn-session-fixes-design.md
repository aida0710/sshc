# VPN経路の修正（寿命・整合性・抽象化）

## 目的

`fix/vpn-session-lifecycle`（eb9a2a82）のレビューで見つかった問題をすべて直す。指摘の一覧は
`~/agent-memory/sshc/vpn-review-2026-09-23.md`にある。

直したあとに成り立たせたいことは次の7つである。

1. どの入口（Terminal、SFTP、CLI、`sshc vpn proxy`、`vpn up`）から使った経路も、同じ規則で
   数えられ、同じ規則で畳まれる。
2. 起動を途中でやめても、誰も止めないコンテナが残らない。
3. 経路を畳むときは、VPN装置へ切断を伝えてから、すぐに終わる。
4. 「経路が用意できた」は、トンネルの相手と実際に話せたことを意味する。
5. 設定と秘密は、ひとつの書き込みとして成功するか、何も変えずに失敗する。
6. 断った理由が、画面とCLIへ具体的に届く。
7. backendをひとつ足すときに触る場所が、そのbackendのファイルに閉じる。

## 変えないもの

- metadata.jsonの`vpnProfiles[]`と`hosts[].vpn`の形。
- Vaultの`vpn`レコードのJSONのキー（`wireguardPrivateKey`、`l2tpPassword`など）。

v0.38.0はすでに公開済みで、metadata.jsonとVaultは端末のあいだで同期される。形を変えると、
古い版が動いている端末で読めなくなる。Goの中の型は作り直すが、保存形式との対応は
エンコードとデコードの関数1か所に閉じ込める。

## 全体の構成

```text
Terminal / SFTP ──┐
                  ├─ vpn.Manager.Dial ── 数える ── コンテナの中継（relay.sock）
CLI / proxy ──────┘         ▲
   │                        │
   └─ engineの中継ソケット ──┘   ← 新設。engineが受けて数え、コンテナへ渡す
```

いまはCLIがコンテナの`relay.sock`へ直接つないでいる。これを、engineが持つ中継ソケットへ
つなぐ形に変える。コンテナの`relay.sock`へつなぐのはengineだけになる。

## 1. 寿命（lifecycle）

### 1.1 engineの中継ソケット（指摘：CLI接続が数えられない）

- `vpn.Manager`は、経路ごとに`<directory>/<profile>/engine.sock`で待ち受ける。受けた接続ごとに
  `Dial`と同じ数え方で借り、コンテナの`relay.sock`へつないで両方向を運ぶ。
- 待ち受けは`Start`の成功で開き、`Stop`で閉じる。
- HTTP APIの`relaySocket`は、このengine側のソケットを指す。CLIのコードは、つなぐ先の
  パスが変わるだけになる。
- コンテナの`relay.sock`はengineだけが使う。ディレクトリは今と同じく0700で、利用者だけが
  読める。
- engineの`Dial`（Terminal、SFTP）はengine側ソケットを経由しない。いま通り
  `countedConnection`を返す。

WebSocketでHTTP APIの上に流す案も比べた。認証をAPIのものに揃えられる利点はある。ただ、
CLIにWebSocketクライアントと中継を足す量に見合わないので採らない。ファイルの権限による
保護は、今と同じ強さである。

ソケットのパスには長さの上限がある（`sun_path`、Linuxで108バイト）。workspaceの位置が
深いと超えうるので、`Start`で先に確かめ、超えたら理由を返して断る。`ErrSocketPath`を
新設する。

### 1.2 数え方と無操作の測り方（指摘：`idleSince`、起動と借用の隙間）

`sessionState`の「使っている数」の意味を、「開いている接続の数」から「開いている接続の数と、
つなぐ途中の数の和」に広げる。

- `Dial`と、engine側ソケットの受け付けは、`Start`を呼ぶ前に`reserve()`で1つ借りる。
  つなげたらそのまま接続の数として持ち、失敗したら`release()`で返す。
  これで、`Start`が返ってから借りるまでの隙間に`StopIdle`が経路を畳むことはなくなる。
- `Start`が成功したら、`idleSince`をその時刻にする。`vpn up`だけで一度も使わなかった
  経路も、起動から10分で畳まれる。
- `idleFor`で`idleSince`が空の場合は、「まだ起動していない」とみなして0を返す。いまと
  同じ扱いだが、上の変更により、起動済みの経路で空になることはなくなる。

### 1.3 鍵を分ける（指摘：`isRunning`が起動を待たされる）

- `running`と`started`は`use`の鍵（数を守っている鍵）の側へ移す。`mutex`は
  「起動と停止を1本にする」ためだけに使う。
- `StopIdle`と`StopAll`は、判断に`use`の鍵だけを使う。起動中の経路は`phase`が空でないので
  畳む対象から外す。

### 1.4 engineの終了と起動の取り消し

- `Manager`に寿命のcontextを持たせる（`vpn.New`で作り、`Manager.Close`で取り消す）。
  `Start`は、呼び出し側のcontextと寿命のcontextのどちらかが終われば止まる。
  engineを終えるときは`Close`を先に呼び、進行中の起動を打ち切ってから`StopAll`へ進む。
- 後始末（`stopContainer`）には、`context.WithoutCancel(ctx)`に`cleanupTimeout`
  （`stopTimeout`＋5秒）を付けたcontextを使う。
  呼び出し側が取り消したあとでも、コンテナは必ず止まる（指摘：取り消しでコンテナが残る）。
- `StopAll`は経路を並べて同時に止める。全体の上限は今の`vpnStopTimeout`（30秒）とする。

### 1.5 前回のコンテナの回収（指摘：uidだけのラベル、起動直後の競合）

- ラベルとコンテナ名に、workspaceの識別子を足す。識別子は`workspace.Root()`の
  SHA-256の先頭12桁である。

  ```text
  io.sshc.vpn.workspace=<12桁>
  sshc-vpn-<profile>-<uid>-<12桁>
  ```

  同じ利用者でもHOMEが違うengineのコンテナには触れない。同じworkspaceで再起動した
  engineは、前回のコンテナを見つけられる。
- `DiscardOrphans`は、今と同じく監視のgoroutineで非同期に動かす。ただし`Manager`が
  「回収が終わった」ことを表すchannelを持ち、`Start`はそれを待ってから進む。
  起動直後の接続が、自分の立てたばかりのコンテナを回収されることはなくなる。
  Dockerが無い場合、回収はすぐ終わったものとして扱う。
- ラベルが変わったので、v0.38.0が立てたコンテナ（uidのラベルだけを持つ）は新しい条件に
  掛からない。回収の条件に「workspaceのラベルを持たず、ownerが自分のuid」を1版だけ
  足しておき、移行が済んだら消す。

### 1.6 残ったソケット

- コンテナが終わると、ホスト側に`relay.sock`と`status.json`が残る。`Status`は、
  コンテナが動いていて、かつengine側ソケットが開いているときだけ`RelaySocket`を返す。
- `Start`の「作り直さずに済むか」の判定からは`relayPresent`を外す。代わりに、engine側
  ソケットの待ち受けとコンテナの稼働を見る。

## 2. コンテナの中（agent）

### 2.1 停止の合図を受ける（指摘：PID 1で10秒待ち、切断を伝えない）

- `docker run`に`--init`を付ける。子プロセスの回収とシグナルの転送はtini（`docker-init`）に
  任せる。
- agentに`trap 'shutdown' TERM INT`を置く。`shutdown`がすることは次の順である。
  1. socatを止める。
  2. 各backendの切断手順を呼ぶ（2.4の`backend_down`）。
     - openconnect: pidファイルのプロセスへSIGINTを送る。openconnectはSIGINTを
       受けると装置へlogoutを送る。
     - L2TP: `ipsec down`のあと`ipsec stop`。
     - WireGuard: 何もしない。状態を持たない方式である。
  3. 終わるのを最大`shutdown_seconds`（5秒）待ち、終わる。
- 待ちの中の`sleep`は、`sleep N & wait $!`の形にする。shは前面の`sleep`が終わるまで
  trapを実行しないので、この形にしないと合図への反応が最大5秒遅れる。
- engine側の`stopTimeout`（10秒）は上限としてそのまま残す。

### 2.2 「用意できた」の意味（指摘：WireGuardの握手を見ていない）

中継を開くのは、backendごとの「相手と話せた」を確かめてからにする。

| backend | 確かめること |
|---|---|
| wireguard | `wg show wg0 latest-handshakes`が0でない。`PersistentKeepalive`で握手が始まるので、待つだけでよい |
| l2tp_ipsec | 今と同じ（`ppp0`にアドレスが付く＝PPPの認証が通った） |
| openconnect | 今と同じ（`vpn0`にアドレスが付く＝装置が設定を配った） |

### 2.3 生きているかの監視（指摘：`inet`を見るだけ）

| backend | 落ちたとみなす条件 |
|---|---|
| wireguard | 最後の握手から`wireguard_stale_seconds`（180秒）を超えた。WireGuardの仕様で、鍵の更新をあきらめる時間（REJECT_AFTER_TIME）である。keepaliveが25秒なので、生きていれば2分ごとに握手が起こる |
| l2tp_ipsec | `ppp0`のアドレスが消えた（今と同じ）。pppdの`lcp-echo`が相手の無応答を検知する |
| openconnect | pidファイルのプロセスが居ない、または`vpn0`のアドレスが消えた |

### 2.4 時間の予算（指摘：段ごとの待ち時間）

- agentへ渡す`attemptSeconds`を、`deadline`（UNIX秒）に置き換える。engineとコンテナは同じ
  カーネルの時計を見るので、絶対時刻で渡してよい。
- agentの待ち（`ipsec up`、PPP、openconnect、握手）は、それぞれ「予算の残り」だけ待つ。
  45秒、15秒といった固定値は無くす。
- `charon`と`xl2tpd`が起動するのを待つ15秒は、予算とは別の「デーモンが立つまで」として
  名前付きの定数に残す。

### 2.5 失敗の理由（指摘6と関連）

- agentは失敗したとき、`/run/sshc-vpn-socket/failure.json`に理由のコードを書く。
  例: `{"reason":"ipsec_negotiation"}`、`"ppp_authentication"`、`"handshake_timeout"`、
  `"target_unresolved"`。
- engineはこれを読み、`ErrSessionFailed`に理由のコードを付けて返す（`SessionFailure`型）。
  HTTP APIは、このコードを`reason`として返す。画面とCLIは、コードを翻訳して表示する。
- 生のログは、今と同じく`sshc vpn logs`と画面のログから見る。APIの応答にログそのものは
  載せない。ログにはIPアドレスやパスが混じり、`problemDetail`の約束（鍵材料や絶対パスを
  含めない）を守れないためである。

### 2.6 agentの分割

`agent.sh`（278行）を、共通の流れとbackendごとの手順に分ける。

```text
container/agent.sh              設定の受け取り、DNS、経路、fail closed、中継、監視、停止
container/backend-wireguard.sh  backend_up / backend_ready / backend_alive / backend_down
container/backend-l2tp.sh
container/backend-openconnect.sh
```

`agent.sh`は、backendの名前から該当ファイルを`.`で読み込み、4つの関数だけを呼ぶ。
`//go:embed`とイメージのタグの計算は、`container/`の全ファイルを対象にしているので、
そのまま追従する。

## 3. Go側のbackendの抽象

### 3.1 `backend` interface（指摘：switchの散在）

`internal/vpn/backend.go`を新設する。

```go
// backend は、トンネルの張り方ひとつぶんの違いである。共通の流れ（コンテナ、
// 中継、経路、fail closed）はここに入れない。
type backend interface {
	// device は、コンテナへ渡すトンネルのデバイスである。
	device() string
	// capabilities は、docker run へ渡す権限の指定である。
	capabilities() []string
	validateSettings(profile Profile) error
	validateSecrets(profile Profile, secrets Secrets) error
	// agentSection は、agent document のうち、この backend の節である。
	agentSection(request agentSectionRequest) (any, error)
	// secretValues は、ログから伏せる値である。
	secretValues(secrets Secrets) []string
	// waitsForApproval は、人の承認を待つ経路かである。
	waitsForApproval(profile Profile) bool
}

var backends = map[BackendName]backend{
	WireGuard: wireGuardBackend{}, L2TPIPsec: l2tpBackend{}, OpenConnect: openConnectBackend{},
}
```

- `wireguard.go`を新設し、`l2tp.go`と`openconnect.go`と同じ並びにする。
  `WireGuardSettings`の検査と設定の本文作りを、`profile.go`と`agentdocument.go`から
  ここへ移す。
- `Validate`、`ValidateSecrets`、`tunnelDevice`、`capabilityArguments`、
  `newAgentDocument`、`redact`、`waitsForApproval`は、`backends`を引く形にする。
  backend名によるswitchは、この表の1か所だけになる。
- 呼び出し側は3つの引数を受け取るので、`agentSection`の引数は構造体
  （`agentSectionRequest{profile, secrets, now}`）にする。

### 3.2 秘密の型（指摘：接頭辞付きの平らなフィールド）

```go
type Secrets struct {
	WireGuard   *WireGuardSecrets   // PrivateKey
	L2TP        *L2TPSecrets        // Password, PSK
	OpenConnect *OpenConnectSecrets // Password, TOTPSecret
}
```

- Vaultに置くJSONのキーは変えない。`EncodeSecrets`と`DecodeSecrets`が、今のキーと
  新しい型を対応させる。
- `SecretsDocument`（JSONの形）を公開する。HTTPの要求（`vpnSecretsRequest`）とCLIの
  送信は、この1つの型を使う。

### 3.3 設定の節を1つに揃える

- 保存するとき（`SaveVPNProfile`）に、backendと違う節を落とす（`normalize`）。
  拒まずに落とすのは、画面が方式を切り替えたあとに古い節を送ってきても、利用者が
  困らないようにするためである。
- `Validate`は、backendに合う節があることに加え、他の節が空であることを確かめる。
- `DNS`の`nil`と空配列は`nil`に揃える。`sameRouteAs`が、意味の同じ設定を
  違うものとして扱わないようにする。

### 3.4 型の重複（指摘：4か所に同じ型）

| 型 | いま | 変更後 |
|---|---|---|
| 保存と通信の形 | `application.VPNProfile`、CLIの`vpnRequestProfile`、CLIの`vpnSession` | `application.VPNProfile`だけにする。CLIもこれを使う |
| 秘密の通信の形 | `vpn.secretsDocument`、`httpserver.vpnSecretsRequest`、CLIの`vpnSecretField` | engineは`vpn.SecretsDocument`だけにする。CLIは秘密をGoの文字列にせず`[]byte`のまま本文を組み立てて送ったあとに消すので、型は共有せず、JSONのキー名（`vpn.SecretKey*`）だけを共有する |
| 一覧の応答 | `httpserver.vpnOverviewResponse`、CLIの`vpnOverview` | httpserverの型を公開し（`httpserver.VPNOverview`など）、CLIもこれを使う |

openapi.yamlの`VPNProfile`は手書きのまま残す。既存の`wire_contract_test.go`に、Goの型を
JSONにしたものがopenapiのスキーマと一致するかの検査を足す。

### 3.5 接続先の照合を1か所に（指摘：比べ方の重複）

- `vpn.Profile.Reaches(address string) bool`を足す。大文字と小文字を区別せず、名前の
  末尾の`.`を無視して比べる。
- engineの`vpnRoute`とCLIの`dialVPNRelay`は、どちらもこの関数を使う。

### 3.6 細かいもの

- `validateResolvers`のエラーを`ErrTarget`から`ErrSettings`へ移す。
- 英字と数字の判定を`isASCIIAlphanumeric`にまとめる。今は3か所にある。
- `VPNProfiles`を名前順に並べる（コメントのとおりにする）。
- `requireOurContainer`と`containerRunning`で、「コンテナが無い」とそれ以外のdockerの失敗を
  区別する。今は、dockerの一時的な失敗も「無い」として扱っている。
- TOTPのコードは、今の30秒の窓の残りが`totpMinimumRemaining`（5秒）より短ければ、
  次の窓まで待ってから作る。渡すあいだに期限が切れないようにするためである。

## 4. 設定と秘密の整合性

### 4.1 ひとつの書き込みにする（指摘：2段階の書き込み、`ErrLocked`の握りつぶし）

既存の`WithConnectionSecretsTransaction`（接続の改名で、Vaultとssh_configを1回の
storageトランザクションで書く）と同じ形を、VPNにも作る。

```go
// internal/secret/vpn.go
func (s *Service) WithVPNSecretsTransaction(
	mutation VPNSecretsMutation, // Set / Rename / Remove のどれか
	commit func(vaultChange *storage.Change) (storage.Result, error),
) (storage.Result, error)
```

- Vaultがロック中なら、何も書かずに`ErrLocked`を返す。
- `commit`は、metadataの変更とVaultの変更を同じ`storage.Request`へ入れて書く。
  成功したときだけ、メモリ上のVaultを差し替える。

### 4.2 手順を持つ場所（指摘：handlerに手順が入っている）

`internal/vpnprofile`を新設し、`Service`に次の操作を置く。httpserverは「入力の検査 →
`vpnprofile.Service`の呼び出し → 応答への変換」だけにする。

| 操作 | 手順 |
|---|---|
| `Create(profile, secrets)` | 名前が無いことを確かめる → 秘密の形式を確かめる（`ValidateSecrets`） → metadataとVaultを1回で書く |
| `Update(profile, secrets)` | 名前があることを確かめる → 保存済みの秘密に、送られた項目だけを重ねる（空の項目は残す。方式が変わったら他の方式の秘密は捨てる） → 重ねた結果を`ValidateSecrets` → 1回で書く |
| `Rename(from, to)` | `from == to`なら何もしない → 名前の形式と重複、Vaultの解錠を確かめる → 経路を止める → 1回で書く |
| `Remove(name)` | Vaultの解錠を確かめる → 経路を止める → metadata（紐付けを含む）とVaultを1回で書く |

- 改名と削除は、確かめられることをすべて確かめてから経路を止める。検査で断る場合に、
  使っている経路を落とさない（指摘：改名で経路が落ちる）。
- 削除は、Vaultがロック中なら断る。秘密だけがVaultに残ると、同じ名前で作り直した経路が
  それを黙って引き継ぐからである。
- 作成では、同じ名前の秘密がVaultに残っていても必ず上書きし、引き継がない。v0.38.0で
  すでに残っている秘密への備えである。

### 4.3 作成と更新を分ける（指摘：upsertによる上書き）

| API | 意味 | 同じ名前がある | 名前が無い |
|---|---|---|---|
| `POST /api/v1/vpn/profiles` | 作成 | 409 `vpn_profile_exists` | 作る |
| `PUT /api/v1/vpn/profiles/:name` | 更新 | 更新する | 404 `vpn_profile_unknown` |

- CLIの`sshc vpn add`は作成を使う。同じ名前がある場合は、`sshc vpn remove`か
  `sshc vpn rename`を案内する。
- 画面の作成フォームは作成を使う。

## 5. 断った理由を届ける

### 5.1 項目ごとの理由（指摘：detailが付かない、画面の検査がずれている）

- `vpn`パッケージの検査エラーを、`FieldError{Field, Reason, Limit}`にする。
  - `Field`: `name`、`target`、`dns`、`wireguard.server`、`wireguard.peerPublicKey`、
    `openconnect.serverCertificate`など、JSONのパス。
  - `Reason`: `required`、`format`、`too_many`、`too_long`、`name_needs_dns`、
    `unreachable_address`など、決まった語。
  - エラーの文言はGoに持たない。
- HTTP APIは`problem`に`field`と`reason`と`limit`を足して返す（openapiの`Problem`を
  拡張する）。画面とCLIは、`vpn.field.<reason>`のキーで翻訳する。
- 規則の正本はGoに置く。`internal/vpn/testdata/profile-cases.json`（入力と、期待する
  `field`と`reason`の組）を作り、Goの検査とTypeScriptのフォーム検査
  （`web/src/vpn/vpnProfileRules.ts`、新設）の両方が、同じ表に対してテストされるようにする。
  片方だけを変えるとテストが落ちる。

### 5.2 起動の失敗

2.5の`reason`を、`vpn_session_failed`の応答に載せる。画面とCLIは、理由とあわせて
「ログを見る」を案内する。

### 5.3 対応が無かったもの

`ErrTargetMismatch`を`vpnProblem`の対応表に足す（400 `vpn_target_mismatch`）。今は
`serviceProblem`に落ちて、汎用のエラーになっている。

## 6. Web UI

- **応答の順番**: `VPNPanel`に世代番号を持たせる。操作を始めるときに世代を進め、古い世代の
  ポーリング結果は捨てる。操作中はポーリングを止める。
- **作成フォーム**: 作成APIを使い、保存に成功したらフォームを空に戻す。
  保存に失敗したときは、秘密を消さずに残す。
- **方式の切り替え**: 方式を変えたら、それまでの方式の秘密をstateから消す。
- **フォームの検査**: `vpnProfileRules.ts`（5.1）で、送る前に項目ごとに理由を出す。
- **拒否コード**: `api/vpn.ts`の一覧は、`vpnRefusals`のキーから作る。
- **用語**: API、型、コンポーネントの名前を次のように揃える。画面の文言の「経路」は、
  pagesで既に使っている利用者向けの呼び名なので変えない。

  | いま | 変更後 |
  |---|---|
  | `VPNSession`（schema、型） | `VPNProfileStatus` |
  | `VPNSessionCard` | `VPNProfileCard` |
  | `VPNRouteChip` | `VPNProfileChip` |

- **方式の表示名**: `vpnBackendLabel(backend)`を1つ作り、フォームとカードの両方で使う。
- **`VPNBindingRow`**: `className`を文字列で置き換えるのをやめ、幅を`size`のpropで受け取る。
- **接続先の食い違い**: HostInspectorの経路の選択肢で、接続先が一致しない経路に注記を付けて
  選べなくする。照合の規則は3.5と同じもの（大文字小文字、末尾の`.`）をTypeScriptに持ち、
  5.1の表と同じ方法でGoとの一致をテストする。

## 7. イメージとDocker

- **aptの固定**: Ubuntuのsnapshot（`snapshot.ubuntu.com`）の日付をDockerfileに書き、
  `apt-get update --snapshot <日付>`で取る。タグが内容から決まるという前提を、パッケージまで
  含めて成り立たせる。日付を上げるのは、基のイメージのdigestを上げるときと同じ作業にする。
- **古いイメージ**: 作り直したあと、今のタグ以外の`sshc-vpn:*`を消す。動いているコンテナが
  使っているイメージは、dockerが消させないので、消せなかった分は無視する。
- **一覧の取得**: `Overview`は経路ごとに3回`docker container inspect`を呼んでいる。
  これを、`docker ps --all --filter label=... --format '{{json .}}'`の1回で全経路の
  状態とラベルを読む形にする。`Available`の判定も、この1回の結果から出す。

## 8. 進め方

振る舞いを変えない整理を先に入れ、その上に修正を載せる。

| 段 | 内容 | 主なテスト |
|---|---|---|
| 1 | 3.1〜3.6（backend interface、秘密の型、型の重複、照合、細部） | 既存のテストが変わらず通ること |
| 2 | 1.1〜1.6（engine側ソケット、数え方、鍵、取り消し、回収、残ったソケット） | `Manager`の単体テスト（偽のdocker）、Docker実機テストに「CLI経由の接続中は畳まれない」「取り消し後にコンテナが残らない」「別workspaceのコンテナに触れない」 |
| 3 | 2.1〜2.6（agent） | Docker実機テストに「停止が1秒台で終わる」「WireGuardの鍵違いは用意できない」「握手が途絶えたら経路が閉じる」「openconnectがlogoutする」 |
| 4 | 4.1〜4.3（整合性） | `vpnprofile.Service`の単体テスト（ロック中の削除と改名、途中失敗で何も変わらない、作り直しで秘密を引き継がない） |
| 5 | 5.1〜5.3（理由） | 共有の表（`profile-cases.json`）に対するGoとTypeScriptのテスト |
| 6 | 6（Web UI） | VPNPanelの順序、作成と更新、フォームの検査のテスト |
| 7 | 7（イメージ） | イメージの作成と、古いタグの削除 |
| 8 | ドキュメント | pages（日英）の機能ページとCLIリファレンス、`docs/manual-acceptance.md` |

段ごとに1〜2コミットとし、`go test ./...`、vet、deadcode、verify-generated、race、
クロスビルド、Webのテストとtsc、pagesのビルド、`SSHC_VPN_DOCKER_TEST=1`を毎段で通す。

## 9. 決めておくこと

設計の中で、利用者に見える振る舞いが変わるもの。

| 項目 | この設計での決め方 | 他の選び方 |
|---|---|---|
| Vaultがロック中の削除 | 断る（解錠を求める） | 設定だけ消し、秘密は残す（今の動き。同じ名前で作り直すと引き継ぐ） |
| `vpn up`だけで使わない経路 | 起動から10分で畳む | 明示的に`vpn down`するまで残す |
| 改名 | 動いている経路を止めてから改名する | 動いたままにし、次に使うときに新しい名前で作り直す（同じ装置へ一時的に2本つながる） |
| aptのsnapshot固定 | 固定する。セキュリティ更新は、sshcの版を上げるときに日付を上げて取り込む | 固定しない（今の動き。同じタグで中身が機械ごとに違う） |
| engine側ソケット | 新設する | WebSocketでHTTP APIの上に流す |

## 10. 実装で設計から変えたこと

| 項目 | 設計 | 実装 | 理由 |
|---|---|---|---|
| 失敗したときのログ | 生のログは`sshc vpn logs`で読む | 用意できなかったコンテナは片付ける前にログを（秘密を伏せて）engineが覚え、`Logs`はコンテナが無ければそれを返す | 片付けるとコンテナのログも消え、理由を読む手段が無くなるため |
| agentの分割 | `backend-l2tp.sh` | `backend-l2tp_ipsec.sh` | backend名から機械的にファイルを選び、名前の対応表を持たないため |
| backend interface | `agentSection(...) (any, error)` | `writeAgentSection(request, *agentDocument) error`、`ownSecrets(Secrets) Secrets` を追加 | agent文書の型を`any`にしないため。更新時に方式の違う秘密を落とすため |
| 更新 | 送られた秘密を重ねる | 同じ。更新にもVaultの解錠が要る | 重ねた結果を`ValidateSecrets`で確かめるには保存済みの秘密を読む必要があるため |
| aptの固定 | `apt-get update --snapshot` | `APT::Snapshot`をapt.confに書き、取得時だけTLSの相手確認を外す | baseのイメージにCA証明書が無く、snapshotはHTTPSでしか配られないため。中身はaptが署名で確かめる |
| テスト用のocserv | — | テスト側のapt取得もDockerfileと同じ方法にした | イメージがsnapshotの設定を持つため |
