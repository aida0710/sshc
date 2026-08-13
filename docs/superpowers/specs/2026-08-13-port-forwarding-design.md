# ポート転送と agent 転送

## 目的

`LocalForward`、`DynamicForward`、`ForwardAgent` を実装する。**開いているものが
見える画面と一緒に。**

B4 で「まだ無い」と印を付けた 3 つである。これでサブプロジェクト B が持ち越した
ものは無くなる。

## この spec の範囲

**B5。** 3 つのキーワードと、それを見せる画面。

移さないもの——`RemoteForward` と `ForwardX11` は B4 で落とすと決めた。

## なぜ画面が要るのか

転送は**このマシンにポートを開く**。開いていることが見えないまま開くのは、
利用者が自分の機械に何を晒しているか分からない状態である。`ssh -L` を打った
人はそれを打ったことを覚えているが、**設定ファイルに書いてある転送は、
接続するたびに黙って開く。**

agent 転送はさらに強い。リモートのプロセスがこちらの鍵で署名を求められる
——**鍵そのものは渡らないが、鍵を使う権利は渡る。**

だから B4 で「画面と一緒に来る」と書いた。

## 決定事項

**1. 転送はセッションの寿命に縛る。** コンソールが閉じれば listener も閉じる。
別に管理する寿命を作らない——**閉じ忘れるものを増やさない。**

**2. bind するのはループバックだけである。**

OpenSSH は `LocalForward 0.0.0.0:8080 …` や `GatewayPorts yes` で他の機械へ
開けるが、**このアプリケーションは開かない。** 常駐するプロセスが同じ機械の
他の面（HTTP サーバー、vault）をループバックに閉じているのと同じ判断である。

ループバック以外の bind address が書かれていたら、**ループバックに束ねて
notice を出す。** 接続そのものは断らない——転送の設定ひとつで繋がらなくなる
方が困る。

**3. bind に失敗しても接続は続ける。** ポートが埋まっているのは普通の
出来事である。**その転送だけを失敗として記録し、端末に理由を書く。**
OpenSSH も同じことをする。

**4. `DynamicForward` は SOCKS5 だけを話す。** SOCKS4 は OpenSSH も受けるが、
いま SOCKS4 しか話せないクライアントを探す方が難しい。**要求されたら足す。**

対応するコマンドは CONNECT のみ。BIND と UDP ASSOCIATE は断る——どちらも
このクライアントが持たない機能である。

認証は「認証なし」だけを受ける。**ループバックにしか開かないので、そこに
到達できる者は既にこのユーザーとして動いている。**

**5. `ForwardAgent` は、agent に到達できるときだけ武装する。**
`SSH_AUTH_SOCK` が無ければ notice を出して接続する——**転送できないことを
理由に接続を断らない。**

**6. 開いているものは、コンソールの一覧と端末の両方に出す。**
一覧では行の下に小さく（`8080 → 10.0.0.5:80`）、端末には接続した時点で
1 行書く。**片方だけだと、スクロールで流れたあと確かめる場所が無い。**

## 構成

```
internal/sshclient/forward.go      listener と、その上を流れる接続
internal/sshclient/socks.go        SOCKS5 の受け口
internal/terminal/terminal.go      Forward と Forwarder（型だけ）
internal/httpserver/terminal.go    View に載せる
web/src/terminal/…                 一覧の行に出す
```

`terminal` パッケージは SSH を知らないままにする。**型だけを置き、
実装は sshclient が持つ**——B2 で `Spec.Open` にしたのと同じ理由である。

### 型

```go
// terminal パッケージ
type Forward struct {
	// Kind は "local" か "dynamic" か "agent"。
	Kind string
	// Listen は、このマシンで開いている場所。agent 転送では空。
	Listen string
	// To は、その先。dynamic と agent では空。
	To string
	// Problem は、開けなかった理由。空なら開いている。
	Problem string
}

// Forwarder は、そのセッションが開いている転送を報告する。
//
// terminal.Process が満たしていなくてよい。ローカルシェルは何も転送しない。
type Forwarder interface{ Forwards() []Forward }
```

### 設定の読み方

`LocalForward` の値は OpenSSH の書式である。

```
LocalForward 8080 10.0.0.5:80                 → 127.0.0.1:8080 から
LocalForward 127.0.0.1:8080 10.0.0.5:80       → 同じ
LocalForward 0.0.0.0:8080 10.0.0.5:80         → ループバックへ束ねて notice
LocalForward [::1]:8080 [fd00::1]:80          → IPv6
```

`DynamicForward` は listen 側だけを取る。

**読めない値は notice を出して飛ばす。** 転送の書式ひとつで接続できなくなる
理由が無い。

## エラー処理

| 起きたこと | 何を返すか |
| --- | --- |
| bind できない（ポートが埋まっている） | その転送を Problem 付きで記録し、端末に 1 行書く |
| 転送先へ繋がらない | その 1 本の接続だけを閉じる。listener は生きている |
| SOCKS の要求が CONNECT でない | その接続だけを断る |
| `SSH_AUTH_SOCK` が無い | notice。接続は続く |
| 値が読めない | notice。その転送だけ飛ばす |

## テスト

**B2 のテストサーバーを拡張する。** `direct-tcpip` は既に扱えるので、
その先に本物の TCP 受け口を置けば、転送されたバイト列が往復することを
端から端まで見られる。

覆うもの。

- `LocalForward` — 開いたポートへ繋ぐとバイト列がリモート側の受け口へ届くこと
- **ループバック以外の bind がループバックに束ねられること**
- 埋まっているポートで、その転送だけが失敗し接続は続くこと
- `DynamicForward` — SOCKS5 の CONNECT が通ること、BIND が断られること
- `ForwardAgent` — リモートから `auth-agent@openssh.com` を開くと、こちらの
  agent の鍵が見えること
- セッションを閉じると listener が閉じること（**同じポートに再び bind できる**）
- 一覧に出る Forward が、実際に開いているものと一致すること

## README の書き換え

「まだ無い」の行が消える。**ループバックにしか開かないこと**と、
agent 転送が何を渡すのかを書く。

## 未解決

- **SOCKS4。** 要求されたら足す
- **`GatewayPorts`。** 他の機械へ開くのは、この製品がする仕事ではない

## 実装者への注意

- **このリポジトリは別のセッションが同時に編集していることがある。** コミット前に
  必ず `git diff --cached` で staged 内容そのものを読む
- Linux は Docker で確かめること
- 新しい文言は `web/src/i18n` の en と ja の両方へ
- **listener は必ず閉じる。** 閉じ忘れたポートは、そのプロセスが死ぬまで埋まる
