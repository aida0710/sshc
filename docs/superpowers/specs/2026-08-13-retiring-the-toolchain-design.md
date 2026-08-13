# 残りの OpenSSH プログラムを退け、落とす機能を確定する

## 目的

B3 のあとに残った OpenSSH のプログラムを見直し、**落とす機能を確定する。**

これは埋め込み SSH バックエンド（サブプロジェクト B）の最後の一歩である。

## この spec の範囲

**B4。** 残る 3 プログラムと、B2 で「まだ無い」と印を付けたキーワードの処遇。

| いま | B4 のあと |
| --- | --- |
| `ssh -Q key`（生成できるアルゴリズムの一覧） | 静的な一覧。**問いの相手が変わった** |
| `ssh-add`（agent への登録） | プロセス内（`x/crypto/ssh/agent`） |
| `ssh-keygen`（FIDO 鍵の生成） | **残す。** x/crypto では作れない |

## 決定事項

**1. アルゴリズムの一覧は、このアプリケーションが生成できるものの一覧である。**

いまは `ssh -Q key` に「インストール済みの OpenSSH は何に対応しているか」を
尋ねている。**それは答えるべき問いではない。** 画面が並べているのは「ここで
生成できる鍵」であり、生成するのは `internal/keys` の x/crypto である。

インストール済みの OpenSSH に尋ねていたのは、生成した鍵がその人の `ssh` で
使えるかを確かめる代理だった。Ed25519・RSA・ECDSA は、対応していない
OpenSSH を探す方が難しい。**代理を立てる必要が無い問いに、代理を立てない。**

FIDO 鍵（`sk-ssh-ed25519@openssh.com`、`sk-ecdsa-sha2-nistp256@openssh.com`）
だけは `ssh-keygen` が要るので、それが見つかるときだけ一覧に載せる。

`fallbackCatalogue` は消える。**尋ねる相手がいなければ答えを縮める**という
仕組みは、尋ねなくなれば要らない。

**2. agent への登録はプロセス内で行う。** `x/crypto/ssh/agent` は
`SSH_AUTH_SOCK` の向こうと直接話せる。B2 の認証が既にそれで鍵を読んでいる。

`ssh-add` を通していたのは、agent のプロトコルを自分で話す手段が無かったから
である。いまはある。**同じソケットに二つの話し方を持たない。**

パスフレーズの扱いは変わる——標準入力ではなく、このプロセスの中で鍵を復号して
から登録する。**外に出る秘密が一つ減る。**

**3. `ssh-keygen` は FIDO 鍵のためだけに残す。** ハードウェアトークンとの
やり取り（PIN、タッチ、libfido2）は x/crypto に無い。`platform.Toolchain` は
`KeyGen()` だけの interface になる。

**4. 落とす機能を確定する。** B2 で「まだ無い」と印を付けたものの処遇。

| キーワード | 決定 | 理由 |
| --- | --- | --- |
| `RemoteForward` | **落とす** | 相手に listen させる。信頼の向きが逆で、サーバー側の `AllowTcpForwarding` と `GatewayPorts` に依存する |
| `ForwardX11` | **落とす** | X サーバーがこのプロセスの向こうに無い。ブラウザの端末に X の窓は出せない |
| `ControlMaster` / `ControlPath` | **落とす** | **プロセス内では意味が無い。** 接続の再利用は `ssh.Client` を持ち回るだけで済み、ソケットも別プロセスも要らない |
| `SendEnv` | **落とす** | 送る値がこのアプリケーションの環境変数であって、利用者のシェルのものではない。**間違った値を送るくらいなら送らない** |
| `CertificateFile` | **落とす** | 証明書を配る仕組みを持つ組織は、その配布と一緒に `ssh` も配っている |
| `LocalCommand` | **落とす** | このアプリケーションは接続のために何も実行しない（B1 からの一貫した判断） |
| `LocalForward` / `DynamicForward` | **B5 で実装する** | 使う人がいて、プロセス内なら実装できる。ただし**開いたポートを見せて閉じる画面が要る**ので、別の spec にする |
| `ForwardAgent` | **B5 で実装する** | 転送と同じ理由。開いていることが見えないまま鍵を貸すのは危ない |

**落とすと決めたものは notice を出し続ける。** 黙って無視すると、利用者が
書いた設定が効いていないことに気づけない。文言は「まだ無い」から
「このクライアントには無い」へ変える——**永久に無いものを、来週来るかのように
言わない。**

**5. `platform.OutputRunner` は残る。** ブラウザの起動、`launchctl`、
`systemctl`、`ssh-keygen` が使う。SSH のためには使わない。

## 構成

```
internal/keys/catalogue.go     ssh -Q key を静的な一覧へ
internal/keys/agent.go         新: x/crypto/ssh/agent を通した登録（新）
internal/platform/keyagent.go  interface はそのまま。実装が変わる
internal/platform/command.go   Toolchain は KeyGen() だけ
internal/platform/process/keyagent.go   削除
internal/sshclient/target.go   notice の表を「このクライアントには無い」へ
```

`platform.KeyAgent` の interface は変えない。**変えるのは実装だけ**であり、
それが interface を持っている理由である。

## エラー処理

| 起きたこと | 何を返すか |
| --- | --- |
| `SSH_AUTH_SOCK` が無い | `ErrAgentUnavailable`（いまと同じ） |
| ソケットに繋がらない | `ErrAgentUnavailable` |
| agent が拒んだ | `ErrAgentRejected`（いまと同じ） |
| 鍵のパスフレーズが違う | `keys.ErrWrongPassphrase` をそのまま |
| `ssh-keygen` が無い | FIDO の項目を一覧に出さない |

## テスト

- **agent** — `x/crypto/ssh/agent` のキーリングを unix ソケットで待ち受けさせ、
  本物のプロトコルで登録・一覧・削除を行う。B2 の認証テストが既に同じ形を持つ
- **カタログ** — 静的な一覧が、`internal/keys` が実際に生成できるものと
  一致すること。**一覧に載っているものは必ず作れる**——載っているのに作れない
  項目は、画面のボタンが必ず失敗するということである
- **FIDO** — `ssh-keygen` が見つからない環境で FIDO の項目が出ないこと
- notice の表 — 落とすと決めたキーワードが notice を出し続けること

## README の書き換え

「残る OpenSSH のプログラムは 3 つ」が 1 つになる。落とすと決めた機能の表を
書く。**「まだ無い」と「無い」を区別して書く。**

## 未解決

- **ポート転送と agent 転送（B5）。** 開いたポートを見せて閉じる画面が要る
- **既知のホスト鍵の種別の優先順位。** いまはサーバーが選ぶ

## 実装者への注意

- **このリポジトリは別のセッションが同時に編集していることがある。** コミット前に
  必ず `git diff --cached` で staged 内容そのものを読む
- Linux は Docker で確かめること
- 新しい文言は `web/src/i18n` の en と ja の両方へ
