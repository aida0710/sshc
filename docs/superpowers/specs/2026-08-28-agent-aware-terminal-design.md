# Agent-aware Terminal design

Date: 2026-08-28
Updated: 2026-08-29

## Summary

sshc の Terminal を、接続先を表示するだけの端末から、そこで動いている coding
agent の状態と再開方法まで理解する端末へ拡張する。

初期対象は Claude Code、Codex、OpenCode とする。Terminal、API、状態 model、PTY上の
event protocol、再接続処理、UIは三製品で共有し、製品差は小さいadapterへ閉じる。

この設計でいう「再開」は、切断前の process memory を復元することではない。

- sshc は同じ pane、terminal session ID、メモリ上の scrollback を維持する。
- 通常の SSH 再接続は新しい remote shell を開く。
- Agent連携が正確なsession referenceを報告していた場合だけ、利用者の明示操作により
  adapterが固定生成したresume processを同じpaneへ開ける。
- `htop` など resume 機構を持たない process は復元しない。任意 process の生存には
  tmux、screen、Mosh、または remote PTY owner が必要であり、この段階では扱わない。

## Implementation status (2026-08-29)

共通coreの最初のsliceを実装済み。

- engine側のbounded OSC decoder、generic state、process generation束縛、TTL、signal version
- Claude Code／Codex／OpenCodeのreference検証と固定resume argv
- same-pane／new-paneの明示resume、同一paneへの入力後の置換拒否、接続identity再検証
- 動的title、利用者titleの固定／解除、作業名とaliasだけのcompact header
- background時だけのattention／completed通知音と、Settingsから明示的に許可するbrowser notification
- APIからopaque referenceを除外する契約とGo／Webの回帰test

連携コードはsshc本体と分離した公開リポジトリ`aida0710/sshc-agent-bridge`に実装済みである。
Claude Code／Codexは同じrepo marketplaceと共通Python reporterを使い、OpenCodeは同じprotocolを
出力するJavaScript pluginを使う。Claude Code／Codexは公式plugin managerによるローカル導入試験、
manifest検証、reporterのunit testまで完了している。OpenCode adapterもunit testとnpm packageの
dry-runまで完了しているが、npm registryへの公開は未実施である。

共通coreはsshc v0.19.0へ収録し、Agent Bridgeを明示導入したTerminalでheader、通知、native
session再開を利用できる。Bridgeの導入はopt-inであり、連携を入れない通常Terminalの動作は変えない。

## Product decision

Terminal を製品の中心に置き、Connections は Terminal を開くための設定、SFTP は
現在の Terminal を補助する道具として扱う。Agent 連携は別 dashboard を作らず、
各 pane の細い header と terminal 内の再接続 banner に収める。

初期 release の原則は次のとおり。

1. 通常の shell は今までどおり動く。Agent 連携は opt-in である。
2. 接続情報は engine が決定し、remote output から上書きできない。
3. Agent 状態は表示の補助情報であり、権限判断や危険操作の根拠にしない。
4. Agent session の再開は利用者が選ぶ。remote output だけを根拠に自動実行しない。
5. Claude Code、Codex、OpenCode固有処理はadapterに閉じ、terminal sessionのlifecycleへ
   混ぜない。
6. ANSI screen の見た目を OCR のように解析する fallback は初期 release に入れない。

## Four layers of continuity

「接続が戻る」を一つの機能として扱わない。

| Continuity | sshc が保持するもの | 新しくなるもの | 例 |
|---|---|---|---|
| Browser attach | SSH process、PTY、pane、scrollback | WebSocket ticket | browser reload |
| Transport reconnect | pane、terminal session ID、scrollback | SSH transport、remote PTY、shell | Wi-Fi 切替、sleep |
| Agent resume | pane、terminal session ID、scrollback、agent session reference | SSH transport、PTY、agent process | Claude Code／Codex／OpenCode の再起動 |
| Remote PTY persistence | 全 process と PTY | client attachmentだけ | tmux、remote daemon |

agent conversation自体は各agentが接続先に保存する。sshcはその会話を保持せず、再開に
必要なopaque referenceだけを一時的に覚える。

現在の sshc は最初の二つを持つ。今回追加するのは三つ目であり、四つ目ではない。

## Terminal header

### Desktop

各paneの上に二つの視覚階層を持つcompact headerを置く。高さは44〜48 pxを目安とし、
一段目は「何の作業か」、二段目は「どこへ接続しているか」を表す。

```text
API認証の修正                                      Codex · 作業中
osaka · via mdx-jamstec-1
```

表示要素の authority を分ける。

- 一段目のagent session name:
  agent integrationがterminal stream上で報告した補助情報
- `osaka`、transport、user、host、port、ProxyJump、encoding:
  engine が解決した接続情報
- agent名、状態、cwd、model:
  agent integration が terminal stream 上で報告した補助情報
- 接続中、host key 確認、認証中、再接続中:
  既存の terminal connection progress

通常headerはaliasだけを表示し、既定から外れる情報だけを追加する。

- transportがSSHなら表示しない。Serial／TelnetなどSSH以外の場合だけ種別を表示する
- `user@host`とportはheaderへ出さずdetail popoverだけに置く
- encodingがUTF-8なら表示しない。Shift_JISなど非既定値だけ表示する
- ProxyJumpを使う場合だけ`via <jump alias>`を表示する。複数hopの完全なrouteはdetailへ置く

したがって一般的なpaneは次の密度になる。

```text
API認証の修正                                      Codex · 作業中
osaka
```

例外がある場合だけ`osaka · via mdx-jamstec-1`、`osaka · Shift_JIS`、
`ttyUSB0 · Serial`のように増える。

一段目のtitle、alias、agent stateは最後まで残す。省略した情報はheader全体を押したdetail
popoverで見られる。titleがaliasと同じ場合は二段へ同じ文字列を重複表示せず、一段目にalias、
二段目は例外情報がある場合だけ表示する。

### Title policy

paneの`displayTitle`は次の優先順位で決める。

1. 利用者が明示的に固定したpane title
2. 現在agentのnative session name
3. resume candidateへ保持した直前agentのsession name
4. connection alias
5. `user@host`
6. `Terminal`

利用者がtitleを固定した後はagentのrenameで上書きしない。固定解除時はその時点のagent session
nameへ戻す。agentの`/rename`等でsession nameが変わった場合は、固定されていないpane title、
session一覧、active browser window titleを同じobservation versionで更新する。アニメーションで
大きく動かさず、文字列だけを置き換える。

agent processの終了やtransport reconnectだけでは作業文脈を失わせない。resume candidateが
有効な間は最後のsession nameをtitleに残し、新しいagent sessionを観測した時点で置き換える。
利用者が「タイトルを接続先に戻す」を選んだ場合は候補名を表示に使わずaliasへ戻す。

session nameはremote由来のuntrusted display valueである。control characterと改行を除去し、
Unicodeを正規化して80 graphemeまでに制限する。HTMLとして解釈せずtextとして描画し、長いtitleは
一行ellipsis、全文はdetail popoverだけに表示する。

### Mobile

一行へすべて詰め込まず、desktopと同じ主従をcompactな二段で表示する。

```text
API認証の修正                         ● 入力待ち
osaka · Codex
```

tapでbottom sheetを開き、完全なsession name、user@host、ProxyJump、encoding、cwd、modelと
再開操作を表示する。agentがいない通常shellでは一段目をaliasとし、二段目には接続状態だけを
表示する。

### Agent state

generic model は四状態だけを持つ。

| State | 意味 | 各adapterが正規化するeventの例 |
|---|---|---|
| `working` | agent が処理中 | prompt submit、tool use、session busy |
| `attention` | 利用者の判断が必要 | permission request、elicitation |
| `ready` | 直前の処理が終わり入力可能 | Stop、session idle |
| `unknown` | agent はいるが現在状態を断定できない | event timeout、未対応 transition |

`done` は agent の意味状態にしない。非表示 pane が `ready` になったことを利用者がまだ
見ていない、という UI の unread flag として別に持つ。pane を focus したら unread を
消す。

色だけに依存せず、icon と表示名を必ず併記する。`attention` は error ではないため、
danger red ではなく warning colour を使う。

### State-based notifications

通知は現在stateではなく、同じprocess generation内のstate transitionから生成する。sessionを
開いた直後に`ready`を観測しただけで完了音を鳴らしてはならない。

| Transition／event | Default notification | Sound |
|---|---|---|
| `unknown`／`ready` → `working` | なし | なし |
| 任意 → `attention` | 「API認証の修正（osaka）が入力を待っています」のようにtitleとaliasを表示 | attention chime |
| `working` → `ready` | 「API認証の修正（osaka）が完了しました」のように表示 | soft completion chime |
| 任意 → `unknown` | なし。headerだけ変える | なし |
| reconnect失敗／process異常終了 | Agent stateとは別のTerminal警告 | warning chime |

初期defaultでは、対象paneがfocus中かつappがforegroundならOS通知も音も出さない。
`attention`だけは利用者が「常に鳴らす」へ変更できる。`ready`は直前の`working`が3秒未満なら
短い処理や起動時heartbeatとして通知しない。observation version単位でdeduplicateし、短時間に
`attention`が連続してもcooldown中は一度だけ鳴らす。

engineのstate machineは前状態と`workingSince`から、内容を持たない
`AgentSignal(kind, version, occurredAt)`を生成する。`kind`は`attention`または`completed`だけとし、
engine自身は音を再生しない。clientはliveで受け取ったsignalだけを通知対象にし、初回attachや
browser reload時は現在versionをbaselineにして古い完了音を再生しない。

通知policyと実際のdeliveryはclient-localにする。pane focus、document visibility、端末ごとの
音量設定はengineが決めない。Desktop shellとAndroidはnative notificationを使い、通常browserは
利用者が許可した場合だけNotifications APIを使う。browserの通知／音声permissionがない場合も
headerとunread badgeは必ず動作する。

browserのpermission promptは初回起動時に自動表示しない。Settingsの「通知を有効にする」を
利用者が押した時だけ`Notification.requestPermission()`を呼び、許可後は確認通知を一度表示する。
許可済みなら同じ場所からtest notificationを送信できる。拒否済みの場合は再要求を繰り返さず、
browserのsite settingsで許可するよう案内する。非対応browserではその状態を表示し、Terminalの
動作は変えない。

初期設定は次を個別に変更できるようにする。

- 入力待ち: OS通知、音、focus中も鳴らすか
- 作業完了: OS通知、音、最小working時間
- 接続警告: OS通知、音
- 全通知の一時muteとpane単位のmute

lock screenへ秘密情報を出さない。通知本文はsanitized display title、alias、`入力待ち`／
`作業完了`だけをdefaultとし、prompt、tool名、cwd、user@host、session referenceを含めない。
通知を押すと該当Terminalとpaneをfocusするが、入力やresume commandは自動実行しない。

## Connection identity

`terminal.Session` は接続開始時に immutable な `ConnectionIdentity` を受け取る。

```text
ConnectionIdentity
  kind             ssh | shell
  alias            optional
  user             optional
  hostName         optional
  port             optional
  jumpAliases      ordered list
  encoding         optional; absent means UTF-8
  hostKeyDigest    optional; SSH authentication後に確定
```

接続中 progress だけから header を作らない。progress は一時的であり、接続完了後も
表示する identity は session が保持する。

API は display に必要な値だけを返す。秘密鍵 path、password、host key 本文、agent の
native session reference は返さない。

## Generic agent model

Terminal session は現在 process generation の agent と、直前 generation から引き継いだ
resume candidate を分けて持つ。

```text
AgentObservation
  adapter          claude | codex | opencode
  state            working | attention | ready | unknown
  cwd              optional display value
  model            optional display value
  sessionName      optional display value
  observedAt
  source           integration
  processGeneration

AgentResumeCandidate
  adapter
  opaqueReference  engine only
  connectionBinding
  sourceGeneration
  observedAt
```

`opaqueReference` は browserへ渡さない。adapter が妥当性を検証し、固定された resume
argv を生成するためだけに使う。

一つのpaneで表示するforeground agentは一つだけとする。各製品のsubagentは同じpaneの
補足情報であり、別paneや別resumable sessionとして数えない。

## Event channel over the PTY

### Why not a local socket

Herdr は agent と multiplexer server が同じ host にあり、hook が Unix socketへ直接
報告できる。sshc の agent は SSH 接続先で動くため、local engine の socket path を
そのまま渡せない。逆向き port forward や remote helperを必須にすると、最初の導入
として重すぎる。

初期 release は terminal byte stream を control channel として共用する。

### Envelope

各agentのmanaged integrationは、共通reporterを介してprivate OSC sequenceを`/dev/tty`へ
書く。暫定identifierは6973とし、実装前にxterm.jsと主要terminalの衝突を再確認して固定する。

```text
ESC ] 6973 ; <base64url JSON> ST
```

payload は version 1 の小さい allowlist JSON とする。

```json
{
  "v": 1,
  "agent": "codex",
  "event": "working",
  "session": "opaque-session-reference",
  "cwd": "/home/aida/projects/sshc",
  "model": "gpt-5.6-codex",
  "seq": 7
}
```

制約:

- encoded payload は最大 4096 bytes
- agent、event、field は allowlist
- session reference は adapter 固有の文字集合と最大長で検査
- cwd と表示値から control character を除き、長さを制限
- prompt、tool input、tool output、transcript path、permission 内容、secret は送らない
- 同一 agent session の `seq` は単調増加し、古い event は捨てる
- `working`／`attention` が一定時間更新されない場合だけ `unknown` へ落とす。正常終了後の
  `ready` は次のevent、`ended`、process終了まで維持する

### Parsing boundary

OSC は browser ではなく engine の terminal pump で解析する。

理由:

- browser が detach していても状態を観測できる
- browser reload後もengine内の状態を返せる
- native session reference をWebへ渡さずに済む
- control sequence をscrollbackとxterm.jsへ流さずに済む

decoder は文字コード変換より前のraw byte列に置き、chunk を跨ぐ sequence を扱う bounded
state machine とする。対象OSC以外の
全byteを元の順序でそのまま ring と stream へ渡す。対象 prefix を認識した後の
malformed／oversized payload は捨て、bufferを無制限に増やさない。

parser、agent state、resume candidate は必ず terminal process generation に束縛する。
切断前の遅延eventが、新しいshellの状態を上書きしてはならない。

## Common Agent Bridge

三製品で共有する実装単位を`Agent Bridge`と呼ぶ。Bridgeは次からなる。

1. remote側でnative hook／plugin eventを共通eventへ正規化する小さいreporter
2. PTY byte streamから共通OSC envelopeを除去・解析するengine decoder
3. `AgentObservation`と`AgentResumeCandidate`をprocess generationへ束縛するsession state
4. 共通のheader、通知、resume UIとHTTP operation
5. 製品ごとの設定配置、reference検証、resume argvだけを持つadapter

remote reporterは製品別に別protocolを発明しない。native eventのJSONまたはtyped eventを読み、
allowlist fieldだけを同じversion 1 envelopeへ変換する。reporter自身はprompt、tool input、tool
output、transcriptを読まない。

engine側adapterの最小contractは概念上次の形とする。

```text
AgentAdapter
  ID()                         claude | codex | opencode
  ValidateReference(string)    opaque referenceの形式検査
  BuildResumeArgv(reference)   固定executableと固定flag
  IntegrationPlan(target)      設定fileとmanaged entryの差分
```

`BuildResumeArgv`へbrowserやremote eventからexecutable、flag、追加argumentを渡さない。
event state machine、connection binding、process replacement、API responseはadapter interfaceへ
入れず、共通coreが所有する。

### Claude Code adapter

Claude Codeは公式hookが`session_id`、`cwd`、event名をJSONで渡し、sessionを
`claude --resume <session-id>`で再開できる。status lineはmodel、context、costなども持つが、
利用者の既存status lineを置換しないため初期integrationには使わない。

| Claude hook | Common event |
|---|---|
| `SessionStart` | identity + `ready` |
| `UserPromptSubmit` | `working` |
| `PreToolUse` | `working` heartbeat |
| `Notification(permission_prompt)` | `attention` |
| `Notification(elicitation_dialog)` | `attention` |
| `Notification(idle_prompt)` | `ready` |
| `Stop` | `ready` |
| `SessionEnd` | `ended` |

### Codex adapter

Codexも公式hookで`session_id`、`cwd`、`hook_event_name`、`model`を受け取れる。非managed hookは
内容のhashに対する利用者のtrustが必要なため、sshcはinstall成功だけで連携済みと表示せず、
Codexの`/hooks`で承認が必要な状態を区別する。resume argvは
`codex resume <session-id>`とする。

| Codex hook | Common event |
|---|---|
| `SessionStart` | identity + `ready` |
| `UserPromptSubmit` | `working` |
| `PreToolUse` | `working` heartbeat |
| `PermissionRequest` | `attention` |
| `PostToolUse` | `working` heartbeat |
| `Stop` | `ready` |
| `SessionEnd` | `ended` |

Codexの`notify`だけではturn完了の通知に偏り、session identityと細かいstate transitionを
統一できないため、Agent Bridgeはlifecycle hookを使用する。

### OpenCode adapter

OpenCodeはglobalまたはproject pluginからtyped eventを購読でき、`session.status`、
`session.idle`、`permission.asked`などを共通stateへ正規化できる。resume argvは
`opencode --session <session-id>`とする。

| OpenCode plugin event | Common event |
|---|---|
| `session.created`／`session.updated` | identity update |
| `session.status` | native statusに応じた`working`／`ready`／`unknown` |
| `permission.asked` | `attention` |
| `session.idle` | `ready` |
| `session.error` | `unknown` |
| `session.deleted` | `ended` |

OpenCodeには既存backendへ`attach`する経路もあるが、これはagent conversation resumeより
強いprocess／server continuityである。初期版では他製品と同じsession resumeだけを扱い、
`opencode attach`はRemote PTY persistence側の後続検討へ分ける。

event coverageが完全でない場合は進行中の`working`／`attention`をTTL後に`unknown`へ移す。
`ready`後にeventがないのは正常なので、時間経過だけでは`unknown`へ移さない。
`ended`は公開stateではなく、現在agentを消して最後の有効なsession identityをresume
candidateへ移すlifecycle eventである。

hook／pluginはagentへcontextを追加せず、判断をblockせず、stdoutへ通常文を出さない。
OSCを書けない非対話sessionでは何もせず成功する。

## Integration installation

連携はagentごとに明示的にinstall／uninstallできなければならない。sshc自身はAgentの設定fileを
編集せず、各Agentの公式plugin manager、marketplace、trust確認へ委ねる。これによりinstall、更新、
削除、コード確認のauthorityをAgent側に残し、sshc固有の`curl | sh`やSFTP経由の設定改変を持たない。

配布単位は公開repo`aida0710/sshc-agent-bridge`とし、初期経路は次のとおり。

| Agent | 導入経路 | 状態 |
|---|---|---|
| Claude Code | repo marketplaceを追加し、`sshc-agent-bridge@sshc`をinstall | 実装・ローカル導入試験済み |
| Codex | repo marketplaceを追加し、`sshc-agent-bridge@sshc`をinstall | 実装・ローカル導入試験済み |
| OpenCode | npm package `sshc-agent-bridge-opencode`を設定へ追加 | package実装済み、npm公開待ち |

Claude CodeとCodexは共通Python reporterを同梱するが、各製品のmanifestとevent mappingは分ける。
OpenCodeはJavaScript pluginから直接同じversion 1 OSC envelopeを`/dev/tty`へ書く。いずれもprompt、
tool input、tool output、permission本文、transcriptを読み取らず、allowlist metadataだけを報告する。

SSH接続先でAgentを動かす場合、pluginはその接続先へ利用者がAgent公式の手順で導入する。sshcの
ローカルUIからremote設定を変更するinstallerは提供しない。未知OSCは通常terminalでは無視されるが、
明示的にBridgeを導入した環境ではsshc以外のterminalにもOSCを書き得ることをREADMEへ明記する。

最初はPython 3と`/dev/tty`を利用できるUnix環境を対象とする。Windows local／Windows OpenSSH targetの
reporterは同じprotocolで追加できるが、Unix版の完了条件には含めない。

## Reconnect and resume behavior

### Network loss

1. 既存のSSH再接続は今までどおり行う。
2. pane、terminal session ID、scrollbackを維持し、境界messageを追加する。
3. 新しいshellが開いた時点でcurrent agent stateは空にする。
4. 切断前の有効なagent identityはresume candidateとして保持する。
5. headerとterminal bannerに「Codex sessionを再開できます」のようにagent名を含めて表示する。
6. 利用者が選ばない限りcommandを送らない。

新しいshellへ利用者がまだ入力していない場合、「同じpaneで再開」はfresh shellを閉じ、
同じterminal sessionとscrollback上で固定resume processへ置換する。すでに入力があった、
startup snippetが動いた、または別processが始まった可能性がある場合は、現在processへ
commandを注入せず「新しいpaneで再開」だけを提示する。

### Explicit reconnect after exit

終了済みpaneにresume candidateがある場合、従来の単一buttonを次の二つへ分ける。

- `<Agent名>を再開`
- `シェルへ再接続`

agentを再開したprocessが終了してもpaneは残り、再びshell再接続を選べる。

### Resume safety

resume candidateは次へ束縛する。

- terminal session ID
- source process generation
- SSH alias
- resolved user、host、port
- verified host key digest
- agent adapter

再接続後のhost identityが一致しない場合はresumeを無効にする。aliasの設定先が変わった
場合に、別hostで古いagent sessionを探してはならない。

各adapterが作るcommandは固定executable、固定flag、厳格に検証したopaque IDだけから
構成する。browserはcommand文字列もsession IDも指定しない。

## API shape

`TerminalSession`へ次を追加する。

```text
connection
  kind
  alias?
  user?
  hostName?
  port?
  jumpAliases[]
  encoding?

presentation
  displayTitle
  titleSource       user | agent | candidate | connection | fallback
  titlePinned

agent?
  kind
  state
  cwd?
  model?
  sessionName?
  resumable
  observationVersion
  signalVersion
  lastSignal?
    kind             attention | completed
    occurredAt
```

`unread`はbrowserごとのfocus状態なのでAPIへ保存しない。Webが最後に見た
`observationVersion`と現在値から計算する。通知音はlive updateで増えた`signalVersion`だけを
対象とし、initial fetchの`lastSignal`を再生しない。

利用者のtitle固定／解除はsession専用operationにする。

```text
PUT /api/v1/terminal/sessions/{id}/title
{
  "title": "API認証の修正" | null
}
```

空文字、control characterだけの値、上限超過を拒否する。`null`は固定解除であり、現在のagent
session nameまたはconnection fallbackを再計算する。agent eventからこのoperationは呼べない。
初期releaseの固定titleはterminal session memoryだけに置き、Saved Workspaceへの永続化は別途
workspace schema変更として扱う。

resumeは専用operationにする。

```text
POST /api/v1/terminal/sessions/{id}/agent/resume
{
  "observationVersion": 12,
  "placement": "same-pane" | "new-pane"
}
```

serverはcandidate、process generation、connection binding、入力有無を再検証する。
`agent_resume_stale`、`agent_resume_unavailable`、`agent_resume_identity_changed`、
`agent_resume_same_pane_busy`を409の固定problem codeで返す。

agent eventをbrowserから投稿するAPIは作らない。authorityはterminal streamだけである。

### Internal ownership

resume用processをHTTP handlerが直接作らない。

- `terminal.Spec`はimmutable connection identityと、adapterを指定して同じ接続先へprocessを
  開くcallbackを持つ
- `terminal.Registry`がpending数、session上限、shutdown、closeとの競合を所有する
- `terminal.Session`がprocess generation、agent candidate、sanitized display title、利用者の
  pinned title、そのgenerationへの利用者入力とengine-generated inputの有無を所有する
- same-pane resumeはmanual reconnectと同じreserve／publish／discard手順を使う
- new-pane resumeは新しいterminal sessionを返し、Webが現在のLive Workspaceへ配置する

この分離により、resumeが既存のconnection limitや「close後にprocessを復活させない」規則を
迂回しない。

## Persistence and privacy

初期releaseではagent observationとresume candidateをengine memoryだけに置く。

- `metadata.json`へ保存しない
- Saved Workspaceへ保存しない
- S3 syncへ含めない
- history／generation backupへ含めない
- scrollbackへOSC payloadを残さない
- engine restart後のagent resumeは提供しない

session referenceは通常credentialではないが、conversationへのhandleであり、一般表示や
diagnostic reportへ含めない。将来engine restartを跨ぐ復元を追加する場合は、device-local
0600 file、接続identity binding、retention、明示削除を別設計で決める。

## Trust boundary

SSH接続先は任意のterminal outputを送れる。したがってcustom OSCも真正なcoding agentだけが
送ったとは証明できない。

この制約から次を守る。

- remote eventは接続identity、認証状態、host key状態を変更できない
- remote eventはvault、SFTP、broadcast、command confirmationへ影響しない
- agent stateは表示と通知だけに使う
- resumeは利用者の明示操作を必要とする
- resume commandはadapterが固定生成し、remoteが任意commandを指定できない
- hidden OSCを使っても、remoteは元から自分のterminal画面を自由に描ける以上のlocal権限を得ない
- malformed eventとunknown adapterは監査可能なdebug logだけに記録し、raw payloadは残さない

将来、agent状態をautomationや自動command実行の根拠に使う場合は、OSCでは足りない。
SSH channelに束縛したauthenticated side channelまたはremote helperを別途設計する。

## Failure behavior

- Integrationなし: connection headerだけを表示し、通常Terminalとして動く
- Integrationが古い: unknown versionを無視し、Terminal outputは壊さない
- Agent eventが`working`／`attention`で止まる: TTL後に`unknown`にする。`ready`は維持し、
  sessionは閉じない
- OSC parser failure: 対象messageだけを捨て、PTY streamを継続する
- agent executableなし: resume processの終了を通常のterminal errorとして表示する
- session not found: agentのmessageをそのままterminalへ表示し、候補を消費してshell再接続を提示する
- connection identity変更: resumeを拒否し、通常shellへの接続だけを許可する
- browser detach: engineがeventを保持し、reattach時に最新状態を返す
- engine restart: 初期releaseでは状態とcandidateを失うことを明示する

## Verification

### Go

- OSC decoderの全chunk境界、BEL／ST終端、malformed、oversized、unknown version
- 対象OSC以外のbyte-for-byte preservation
- control payloadがring、stream、diagnosticへ到達しないこと
- process generationを跨ぐ遅延eventの拒否
- sequence逆転と、`working`／`attention`だけに適用するTTLのstate transition
- `working`時間、signal version、attention cooldownのtransition table
- resume candidateのconnection／host key binding
- user input後のsame-pane replacement拒否
- close、reconnect、resume競合でprocessが復活しないこと
- session referenceがAPIへ出ない契約test
- agent rename、resume candidate、pinned title、alias fallbackのtitle precedence table
- remote session nameのcontrol character、改行、長さ、Unicode正規化
- parser fuzz target

### Web

- desktop headerでSSH、user@host、port、UTF-8を抑制し、例外情報だけ追加すること
- detail popoverではuser@host、port、完全なProxyJump routeを確認できること
- desktop headerのoverflow
- mobile compact headerとbottom sheet
- session name変更時のpane／session一覧／window title同期と、pinned title非上書き
- titleとaliasの重複抑制、ellipsis、通常shell fallback
- working／attention／ready／unknownとunread
- focus／visibility／mute別の通知policyと、reload時に古いsignalを鳴らさないこと
- 通知本文が`displayTitle（alias）`となり、prompt、cwd、user@host、session referenceが
  入らないこと
- connection progressとagent stateの同時表示
- same-pane／new-pane resume affordance
- stale／identity mismatch problemの表示
- integrationが無い通常Terminalの回帰

### Integration

- fake PTY processが分割したOSCを出し、build済みengine APIへstateが現れること
- browser detach中のeventをreattach後に見られること
- transport reconnect後に同じsession IDとscrollbackを保ちcandidateだけが残ること
- fixed adapter argv以外を実行できないこと
- Claude／Codex marketplace manifestとOpenCode npm packageが各公式schema／loaderで受理されること
- Claude／Codexを公式plugin managerでinstall／uninstallでき、既存設定を直接変更しないこと
- 三adapterが同じnative lifecycleを同じcommon stateへ正規化するcontract test
- 各adapterが固定resume argv以外を生成できないtable test
- Codex integrationの未承認状態を「設定済み・承認待ち」として扱うこと
- Shift_JISなどの出力変換より前にASCII OSCを除き、周囲の文字列を正しく変換すること

実Claude Code／Codex／OpenCode、実remote host、permission prompt、network断はmanual
acceptanceへ残す。
実資格情報、prompt本文、session IDを検証記録へ保存しない。

## Delivery order

### Phase 1 — Connection header

現在のTerminalへengine由来のaliasを主表示し、ProxyJump、非UTF-8、SSH以外のtransportだけを
例外表示する。`user@host`、port、完全なrouteはdetailへ置く。Agent protocolはまだ入れない。
このphaseだけでも接続先を理解でき、通常時は静かなTerminalになる。

### Phase 2 — Common Agent Bridge + three observation adapters

bounded OSC decoder、generic agent model、共通reporter、Claude／Codex／OpenCode integration、
dynamic session title、二層header、unread notificationを追加する。resumeはまだ行わない。
三adapterを同時に通すことで、共通coreへClaude固有前提が入り込んでいないことを初期段階で
検証する。

### Phase 3 — Common agent session resume

resume candidate、connection binding、same-pane process replacement、new-pane fallbackと三製品の
resume argv adapterを追加する。自動resumeは行わない。

### Phase 4 — Adapter hardening

agent version差、Codex hook trust、OpenCode plugin API変更をfixtureとcapability detectionで
吸収する。配布は各Agentの公式plugin managerへ委ね、共通protocolとTerminal APIは変更しない。

### Later — Process persistence

tmux attach支援、Mosh、またはauthenticated remote PTY ownerを比較する。任意processの
永続化はAgent resumeから独立した設計とreleaseにする。

## Rejected alternatives

### Automatically replay the last command

shell historyの最後が安全なresume commandとは限らず、`sudo`、削除、対話processへの入力を
再実行し得るため採用しない。

### Infer agent session from visible text only

theme、version、terminal幅、翻訳で壊れ、正確なsession IDも得られない。明示integrationが
無い場合はresumableを名乗らない。

### Persist every agent reference immediately

engine memoryだけという現在のscrollback境界を不意に広げ、別hostへの誤resumeやprivacy上の
retentionを生む。engine restartを跨ぐ復元は後続設計に分ける。

### Build a remote daemon first

任意processの完全な生存には有効だが、install、upgrade、authentication、socket ownership、
cleanup、Windows対応が一度に必要になる。各agent自身のsession resumeを先に利用する方が
小さく価値を検証できる。

## External references

- Claude Code session resume: https://code.claude.com/docs/en/sessions
- Claude Code hooks: https://code.claude.com/docs/en/hooks
- Claude Code status line fields: https://code.claude.com/docs/en/statusline
- Codex lifecycle hooks: https://learn.chatgpt.com/docs/hooks
- Codex CLI resume: https://learn.chatgpt.com/docs/developer-commands?surface=cli
- OpenCode plugin events: https://dev.opencode.ai/docs/plugins/
- OpenCode CLI session resume: https://dev.opencode.ai/docs/cli/
- Herdr integration authority and session identity: https://herdr.dev/docs/integrations/
- Herdr session state and restore: https://herdr.dev/docs/session-state/
