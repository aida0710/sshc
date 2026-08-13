# 外部の ssh を退ける

## 目的

接続のために OpenSSH のプログラムを起こすのをやめる。`SSH_ASKPASS` の一式を
撤去する。

これは埋め込み SSH バックエンド（サブプロジェクト B）の第三歩である。B1 で
値を自分で決められるようになり、B2 で埋め込みターミナルが自分で接続する
ようになった。B3 は残りの 4 経路を移し、**その結果として要らなくなるものを
消す。**

## この spec の範囲

**B3 のみ。** 接続に関わる 4 経路と、askpass の一式。

| 経路 | いま | B3 のあと |
| --- | --- | --- |
| 認証テスト | `ssh -v -o BatchMode=yes` | プロセス内 |
| 公開鍵のリモート登録 | `ssh alias 'sh -c …'` | プロセス内 |
| ホスト鍵の取得 | `ssh-keyscan` | プロセス内 |
| CLI の `sshc <接続先>` | `ssh` を exec | プロセス内 |

移さないもの——`ssh-keygen`（FIDO 鍵）、`ssh-add`（agent 登録）、
`ssh -Q key`（アルゴリズム一覧）は残る。**あれは接続ではない。** それらを
どうするかは B4 で決める。

`~/.ssh/config` は正本のまま。CLI も残る——**移すのは中身であって、コマンドでは
ない。**

## なぜいま消せるのか

`SSH_ASKPASS` は、**OpenSSH がパスフレーズを尋ねる相手を、別のプログラムとして
しか指定できないから**存在していた。ヘルパーを exec し、ヘルパーが localhost の
エンドポイントで単回トークンを引き換え、パスフレーズを標準出力へ書き、
OpenSSH がそれを読む。

自分で SSH を話すなら、尋ねる相手は自分である。**プログラムも、トークンの
往復も、凍結した設定ファイルも要らない。**

プロンプト文字列の照合（`Answerable`、`KeyPassphraseAnswerable`）も同じ理由で
消える。あれは「OpenSSH がいま何を尋ねているか」を知る手段がプロンプト文しか
なかったから書かれた。自分で尋ねるなら、何を尋ねているかは自明である。

## 決定事項

**1. 4 経路は B2 の `internal/sshclient` を使う。新しい SSH の話し手を作らない。**
`Dialer` に、対話セッション以外の 3 つの使い方を足す。

```go
// Probe は、接続して認証だけを試し、チャンネルを開かずに閉じる。
func (d Dialer) Probe(ctx context.Context, target Target) (Probe, error)

// Run は、リモートで 1 つのコマンドを走らせる。端末は要求しない。
func (d Dialer) Run(ctx context.Context, target Target, command string, stdin []byte) (Output, error)

// HostKeys は、認証せずにホスト鍵だけを集める。
func ScanHostKeys(ctx context.Context, dial DialFunc, address string) ([]ssh.PublicKey, error)
```

**2. 認証テストは「通ったか」をそのまま答える。`-v` の出力は読まない。**
プロセス内では、通ったかどうかは握手の結果そのものである。文字列から推測する
必要が無い。**どの方式で通ったかも言える**——いまは `-v` の行を読んで推測して
いる。

いまの約束は守る。**尋ねない。** 保存済みパスフレーズは使うが、端末の問いは
出さない（`Prompter` を渡さない）。ホスト鍵は `StrictHostKeyChecking=yes` 相当
——未知のホストは断る。転送も `LocalCommand` も、そもそもこのクライアントに無い。

`HardeningOptions` は消える。**あれは外部の ssh に「するな」と言うための一覧
だった。** このクライアントにはその機能が無いので、言う相手がいない。

**3. ホスト鍵の取得は、鍵種別ごとに握手を始めて中断する。**
`ssh-keyscan` がしているのと同じことである。`HostKeyAlgorithms` を 1 つに
絞って接続し、`HostKeyCallback` で鍵を受け取ったところで断る——認証には
進まない。**鍵を集めるのに資格情報は要らない。**

対象の種別は `ssh-keyscan` の既定と同じ並びにする。得られなかった種別は
黙って飛ばす——サーバーが持っていないだけである。

**4. リモート登録は `exec` チャンネルで走らせる。鍵は stdin を通る。**
いまと同じ約束である。**公開鍵が argv に乗ることは決してない。**
`writeConfigSnapshot` は消える——凍結した設定を `ssh -F` に読ませる必要が
無くなるからである。

**5. CLI は端末を raw モードにして、自分でつなぐ。**
`golang.org/x/term`（既に直接の依存）で `os.Stdin` を raw にし、`SIGWINCH` で
大きさを送り直す。終了時には必ず元に戻す——**戻さないと、そのシェルが以後
壊れたままになる。**

`sshc <接続先>` の見た目は変わらない。変わるのは、そこで動いているものである。

**6. `/cli/connect` は残すが、返すものが変わる。**

| | いま | B3 のあと |
| --- | --- | --- |
| 返すもの | askpass の単回トークン、ヘルパーのパス、凍結した設定 | その接続に使う鍵のパスフレーズ |
| 認証 | handoff secret | handoff secret（変わらず） |
| 使う相手 | ssh が exec するヘルパー | sshc 自身 |

**localhost を通るものは変わらない。** いまもパスフレーズはこの経路を通って
いる——ヘルパーが引き換えて OpenSSH へ渡している。往復が 1 回減り、
間に立つプログラムが 1 つ消えるだけである。

トークンを単回にしていたのは、**引き換える相手が別のプログラムだった**からで
ある。要求そのものが応答を受け取るなら、発行と引き換えを分ける理由が無い。

sshc が走っていない、vault が施錠されている、その alias に保存が無い——
どれでも CLI は接続する。**端末で尋ねればよい。** これは B2 で作った橋渡しが
そのまま使える。

**7. 消すもの。**

```
cmd/sshc/askpass.go                      ヘルパーの本体
internal/httpserver/password.go の Askpass エンドポイントと prompt 照合
internal/platform/interactive.go         InteractiveSSH と FreezeSSHConfig と 5 つの変数
internal/diagnostics/authentication.go の HardeningOptions
internal/remotekey/register.go の writeConfigSnapshot
platform.Toolchain.KeyScan
secret.Service の askpass トークン一式（IssueKeyPassphraseToken、RedeemCredential）
Options.Answerable、Options.KeyPassphraseAnswerable、Options.AskpassHelper
```

**8. 落とす機能。** プロセス内のクライアントに無いものは、その経路でも無い。

- **認証テストの `-v` の詳細**。どの鍵が試され、どこで拒まれたかの OpenSSH の
  逐次ログは出なくなる。代わりに「どの方式で通ったか／どれも通らなかったか」を
  答える。**推測ではなくなる。**
- **`ssh-keyscan` の証明書と DNS の扱い**。`-c` も `-D` も持たない
- **CLI から `ProxyCommand` を使う設定へ接続すること**。B2 と同じ判断である。
  `~/.ssh/config` は正本のまま残るので、`ssh` で直接繋げる

## 構成

```
internal/sshclient/
	probe.go      認証だけを試す（新）
	exec.go       リモートで 1 コマンド走らせる（新）
	scan.go       ホスト鍵を集める（新）
	tty.go        ローカル端末を raw にして繋ぐ（新、CLI 用）
```

`internal/diagnostics`、`internal/remotekey`、`internal/knownhosts` は
`Runner` と `Toolchain` を手放し、`sshclient.Dialer` を受け取る。

### 認証テストの答え

```go
type Probe struct {
	// Method は通った認証方式。空なら、どれも通らなかった。
	Method string
	// Banner はサーバーが送った文言。空でありうる。
	Banner string
	// Tried は試した方式の並び。
	Tried []string
}
```

失敗の理由は、いまと同じく上限つきの文字列で返す。**ホームディレクトリのパスは
`~` に置き換えてから**——利用者のアカウント名を応答本文へ持ち出さない。

### CLI の端末

```go
// Attach は、このプロセスの端末を SSH のセッションへ繋ぐ。
//
// 戻るのはセッションが終わったときである。端末の状態は必ず元に戻す。
func Attach(ctx context.Context, process terminal.Process, in *os.File, out io.Writer) (int, error)
```

`in` がテレタイプでないとき（パイプの中で走っているとき）は raw にしない。
**大きさも問い合わせない。** その場合でも読み書きは通る。

## エラー処理

| 起きたこと | 何を返すか |
| --- | --- |
| 解決器が拒んだ | いまと同じく、その理由 |
| `ProxyCommand` がある | 断る。CLI は「端末から ssh で繋げます」と添える |
| 認証テストで未知のホスト | 断る。Known Hosts 画面で追加できると添える |
| CLI が sshc に届かない | 保存済みパスフレーズ無しで続行し、端末で尋ねる |

## テスト

**B2 のテストサーバーをそのまま使う。** プロセス内の SSH サーバーが既に
公開鍵・パスワード・keyboard-interactive・`exec`・`direct-tcpip` を持っている
ので、4 経路すべてを本物の握手で検査できる。

覆うもの。

- 認証テスト——通る、通らない、未知のホストで断る、**端末で尋ねないこと**
- ホスト鍵の取得——複数種別、サーバーが持たない種別を飛ばすこと、認証しないこと
- リモート登録——鍵が stdin を通ること、**argv に乗らないこと**、終了コード
- CLI——テレタイプでない入力で raw にしないこと、終了コードが伝わること、
  端末の状態が必ず戻ること
- **`internal/acceptance` に「どの経路も外部プロセスを起こさない」を 1 本置く。**
  いまは埋め込みターミナルについてだけ言っている

## README の書き換え

「SSH 実行の境界」がまた変わる。**このアプリケーションが接続のために起こす
外部プログラムは無くなる。** `ssh-keygen` と `ssh-add` と `ssh -Q key` が
残ることと、その 3 つが接続ではないことを書く。`SSH_ASKPASS` の節と、
`make install` が絶対パスを埋め込む理由の節も変わる。

## 未解決

- **`ssh-keygen`（FIDO 鍵）、`ssh-add`（agent 登録）、`ssh -Q key`。** B4 で
  決める。FIDO は x/crypto では作れないので、これは残る可能性が高い
- **既知のホスト鍵の種別の優先順位。** いまはサーバーが選ぶ。設定で変えたい人が
  現れたら考える

## 実装者への注意

- **このリポジトリは別のセッションが同時に編集していることがある。** コミット前に
  必ず `git diff --cached` で staged 内容そのものを読む
- Linux は `GOOS=linux go vet` では確かめられない。Docker で走らせること
- 新しい文言は `web/src/i18n` の en と ja の両方へ入れる
- **秘密をログにも応答にも出さない。** 端末の状態を戻す処理は `defer` に置き、
  panic でも通ること
