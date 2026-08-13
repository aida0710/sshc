# プロセス内 SSH クライアント

## 目的

埋め込みターミナルの SSH セッションを、外部の `ssh` を起こすのではなく、この
プロセスの中で話す。

これは埋め込み SSH バックエンド（サブプロジェクト B）の第二歩である。B1 で
「この alias に接続すると何が使われるか」を自分で決められるようになった。
B2 はその値を使って自分で接続する。

## この spec の範囲

**B2 のみ。** 対話ターミナルの経路だけをプロセス内へ移す。

移さないもの——認証テスト（`internal/diagnostics`）、公開鍵のリモート登録
（`internal/remotekey`）、`ssh-keyscan`（`internal/knownhosts`）は、B2 のあとも
外部の `ssh` を起こし続ける。**askpass ヘルパーも残る。** それらの移行と
askpass の撤去は B3 である。

`~/.ssh/config` は正本のまま。CLI も残る。

## なぜプロセス内なのか

- **Android には `ssh` が無い。** 外部の実行ファイルに接続を委ねる設計は、そこで破綻する
- **PTY を確保する必要がなくなる。** いまの SSH セッションは、`ssh` を PTY の中で
  起こし、その PTY を WebSocket へ繋いでいる。プロセス内なら PTY は要らない——
  SSH のチャンネルがそのまま端末である。`creack/pty` が要るのはローカルシェルだけになる
- **B1 で決めた値を、そのまま接続に使える。** いまは値を決めたあと、その値を
  `ssh` に「もう一度読ませる」ために設定ファイルを凍結して `-F` で渡している

## 決定事項

**1. `golang.org/x/crypto/ssh` を使う。自前の SSH は書かない。** すでに直接の
依存であり（鍵の生成と検査が使っている）、新しい依存は増えない。「薄いカスタム
実装」は暗号のプロトコルには当てはめない。

**2. 継ぎ目は `terminal.Spec` に一つ足すだけにする。**

```go
type Spec struct {
	// ...
	// Open は、この セッションの Process を自分で作る。設定されていれば
	// Starter は使われない。SSH はここを通る——プロセス内で話すので、
	// 確保する PTY が無い。
	Open func(Size) (Process, error)
}
```

`Registry` は SSH を知らないままにする。あそこが持っているのはセッションの
一覧とリングバッファであり、向こうにいるものが何かは知らなくてよい。

**3. 対話プロンプトを端末へ橋渡しする。** 接続の途中で人に尋ねることが 4 つある。

| 尋ねること | いつ |
| --- | --- |
| 未知のホスト鍵を受け入れるか | known_hosts に無いホスト |
| 鍵のパスフレーズ | vault に保存されていない鍵 |
| パスワード | password 認証 |
| keyboard-interactive の質問 | 2FA など |

**この 4 つは同じ仕組みで扱う。** セッションを先に作り、ハンドシェイクはその
出力ストリームへ書き、入力ストリームから読む。これは本物の SSH クライアントが
していることそのものである。**仕組みを 4 つ作らない。**

橋渡しがあるので、`SSH_ASKPASS` に相当するものは B2 の中に要らない。ただし
askpass ヘルパー自体は B3 まで残る——外部の `ssh` を起こす経路がまだあるからだ。

**4. ホスト鍵が食い違ったら、必ず断る。尋ねない。** known_hosts にあって鍵が
違うのは中間者攻撃の形そのものである。ここだけは人に判断させない。未知のホストは
尋ねる（上の表）。受け入れた鍵は `internal/knownhosts` の Service を通して
`known_hosts` へ書く——**書く場所が二つある状態を作らない。**

**5. `ProxyJump` はプロセス内で辿る。`ProxyCommand` は断る。** 前者は SSH の
チャンネルの上に次の接続を載せるだけで、プログラムを起こさない。後者はプログラムを
起こす。B1 が `Match exec` について決めたことと同じ理由で断る——**この
アプリケーションは接続のために何も実行しない。**

**6. honour するものと、notice を出すだけのものを分ける。**

| 扱い | キーワード |
| --- | --- |
| honour する | `HostName` `Port` `User` `IdentityFile` `IdentitiesOnly` `ProxyJump` `SetEnv` `ServerAliveInterval` `ServerAliveCountMax` `RemoteCommand` `RequestTTY` `ConnectTimeout` `StrictHostKeyChecking` `PubkeyAuthentication` `PasswordAuthentication` `KbdInteractiveAuthentication` `PreferredAuthentications` |
| 断る | `ProxyCommand` `Match exec` `CanonicalizeHostname`（B1 が既に断る） |
| notice を出して接続する | `LocalForward` `RemoteForward` `DynamicForward` `ForwardAgent` `ForwardX11` `ControlMaster` `ControlPath` `LocalCommand` `SendEnv` |

**接続先が変わるものは断り、機能が足りないだけのものは notice にする。**
`LocalForward` が無いことを理由に接続そのものを断ると、転送を使っていない日にも
繋がらなくなる。逆に `ProxyCommand` を黙って無視すると、利用者が意図した経路とは
別の経路で繋いでしまう。

`SendEnv` が notice 側なのは、送る値がこのアプリケーションの環境変数であって、
利用者のシェルのものではないからである。**間違った値を送るくらいなら送らない。**
`SetEnv` は設定に書かれた値そのものなので honour する。

**7. 落とす機能を最終決定するのは B4 である。** 上の notice の表は B2 が
「まだ無い」と言うためのものであり、「永久に無い」と言うためのものではない。

## 構成

```
internal/sshclient/          新しいパッケージ
	client.go     Dial: 設定の値ひとつから接続ひとつ
	auth.go       認証方式の組み立てと順序
	hostkey.go    known_hosts との突き合わせ
	session.go    terminal.Process の実装（PTY を持たない）
	prompt.go     端末ストリームへの橋渡し
	jump.go       ProxyJump の連鎖
```

`internal/httpserver/terminal.go` の `spec()` は、SSH の枝で
`platform.InteractiveSSH` を呼ぶ代わりに `sshclient.Dialer` を組み立てて
`Spec.Open` へ渡す。

### 入力の形

```go
// Target は、ひとつの接続に要る値の全体である。
//
// effective.Resolve の答えから組み立てる。**この構造体は ~/.ssh/config を
// 読まない。** 読むのは B1 の解決器であり、ここへ来るのはその答えだけである。
type Target struct {
	Alias      string
	HostName   string
	Port       string
	User       string
	Identities []string // 解決済みの絶対パス
	IdentitiesOnly bool
	Jump       []Target // ProxyJump の連鎖。手前から順に
	SetEnv     map[string]string
	KeepAlive  time.Duration
	RemoteCommand string
	Timeout    time.Duration
	Strict     string // StrictHostKeyChecking の値
	Methods    Methods // どの認証方式を許すか
}
```

`Identities` が解決済みの絶対パスなのは、`~` とトークンの展開を
`internal/effective` が既に持っているからである。**同じ展開を二度書かない。**

### 認証の順序

`PreferredAuthentications` が書かれていればそれに従う。書かれていなければ
`publickey`、`keyboard-interactive`、`password` の順——OpenSSH の既定と同じ。

`publickey` は次の順に試す。

1. `IdentityFile` に書かれた鍵（書かれた順）
2. `SSH_AUTH_SOCK` の agent が持つ鍵（`IdentitiesOnly yes` なら飛ばす）

`IdentityFile` が書かれておらず agent も無い接続は、OpenSSH の既定の探索順
（`~/.ssh/id_ed25519` など）を**持たない**。B1 が既定を持たないと決めた理由が
そのまま当てはまる。鍵が要る接続で鍵が指定されていなければ、その旨を notice に
出して次の方式へ進む。

パスフレーズは、まず vault を見て、無ければ端末で尋ねる。**尋ねた答えは保存
しない。** 保存は Secrets 画面の仕事であり、接続の途中で黙って永続化しない。

### ホスト鍵

`internal/knownhosts` の `ParseFile` と `MatchesHost` で突き合わせる。
`x/crypto/ssh/knownhosts` は使わない——**同じ問いに答えるパーサーを二つ持たない。**
あの画面が読んでいるファイルと、接続が読むファイルは同じでなければならない。

| 状態 | `StrictHostKeyChecking` | 結果 |
| --- | --- | --- |
| 一致 | 何でも | 接続する |
| 不一致 | 何でも | **断る。尋ねない** |
| 未知 | `no` / `accept-new` | 受け入れて書く |
| 未知 | 既定 / `ask` | 端末で尋ねる |
| 未知 | `yes` | 断る |

### terminal.Process の実装

```go
Read/Write   → SSH のセッションチャンネルの stdout/stderr と stdin
Resize       → window-change リクエスト
Hangup       → チャンネルを閉じ、次に接続そのものを閉じる
Wait         → リモートの終了コード。接続が落ちた場合はその理由
Close        → 接続を閉じる
```

`Hangup` に SIGHUP は無い。**プロセスが無いからである。** 同じ意図——「向こうに
終わってほしい」——をチャンネルを閉じることで表す。

## エラー処理

接続が始まらなかった理由は、終了済みセッションとして一覧に残す。いまと同じで
ある。`ssh` が接続できなかった理由が読めるのは、そこだけだからだ。

| 起きたこと | 何を返すか |
| --- | --- |
| B1 の解決器が拒んだ（`Match exec` など） | セッションを作らず、その理由を返す |
| `ProxyCommand` が設定されている | セッションを作らず、断る理由を返す |
| ホスト鍵が食い違った | セッションは作り、端末へ理由を書いて終了させる |
| 認証がすべて失敗した | 同上。試した方式を並べる |
| 接続が落ちた | `ExitInfo` に理由を入れる |

**「セッションを作らない」と「作って理由を書く」を分けるのは、前者が設定の
問題であり、後者が接続の出来事だからである。** 設定の問題は接続画面が既に
表示できる。接続の出来事は端末にしか出せない。

## テスト

**プロセス内の SSH サーバーを立てて、本物のハンドシェイクで検査する。**
`x/crypto/ssh` はサーバー側も持っているので、外部バイナリは要らない。

待ち受けるのは 127.0.0.1 の任意ポートである。**`net.Pipe` では動かない**——
あれは同期的で、書き込みが対応する読み取りまで返らない。SSH の版文字列の交換は
両側が先に書くので、その場で相互に固まる。`x/crypto/ssh` 自身の検査が
ループバックのソケットを使っているのは同じ理由である。

これが B2 の中心的なテスト方針である。いまの端末のテストは `Starter` を
差し替えて実プロセスを避けているが、SSH は**プロトコルそのものが検査対象**なので、
偽物に置き換えると何も確かめられない。

覆うもの。

- 公開鍵認証（パスフレーズ無し、vault のパスフレーズ、端末で尋ねたパスフレーズ）
- agent 認証（`SSH_AUTH_SOCK` をプロセス内の agent へ向ける）
- password と keyboard-interactive（端末への橋渡し）
- ホスト鍵——一致、不一致（断ること）、未知（尋ねること、`accept-new` で書くこと）
- `ProxyJump` の連鎖（サーバーを 2 つ立て、手前が奥へ direct-tcpip を通す）
- window-change が届くこと
- リモートの終了コードが `ExitInfo` に入ること
- `ProxyCommand` を持つ設定が接続を断ること

**実リモートへは接続しない。** いまの約束と同じである。

e2e は既存の `terminal.spec.ts` を拡張する。ローカルシェルの検査はそのままで、
プロセス内 SSH サーバーへ繋ぐ検査を足す。

## README の書き換え

「SSH 実行の境界」が変わる。**外部の `ssh` を起こすのは、認証テスト・公開鍵の
リモート登録・`ssh-keyscan` の 3 つだけになる**（B3 でそれも無くなる）。
対話セッションが PTY を確保しなくなったことと、上の notice の表を書く。

## 未解決

- **ポート転送。** B4 で決める。プロセス内なら実装そのものは難しくない
  （`ssh.Client.Listen` と `Dial`）が、UI が無い
- **ControlMaster。** プロセス内では意味が変わる。接続の再利用は
  `ssh.Client` を持ち回るだけで済むので、あの設定は要らなくなる可能性が高い
- **証明書（`CertificateFile`）。** `x/crypto/ssh` は扱えるので難しくないが、
  使っている人が居ない。B4 で決める

## 実装者への注意

- **このリポジトリは別のセッションが同時に編集していることがある。** コミット前に
  必ず `git diff --cached` で staged 内容そのものを読む
- Linux は `GOOS=linux go vet` では確かめられない。Docker で走らせること
- 新しい文言は `web/src/i18n` の en と ja の両方へ入れる
- **秘密をログにも応答にも出さない。** パスフレーズ、パスワード、
  keyboard-interactive の答えは、端末のストリームにも書き戻さない
