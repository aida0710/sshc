# 接続ごとのVPN経路（VPNセッション）

## 目的

ホストの既定経路・DNS・VPNクライアントを変えずに、**選んだSSH接続だけ**を専用のVPNへ通す。

得たいのは次の状況である。ホストがVPN1へ繋がっている状態のまま、VPN2の中にしか居ない
ホストへ`sshc`から接続する。ホスト側のVPNクライアントを切り替えない。片方のVPNのために
もう片方を諦めない。

出自は`~/tohoku-projects/vpn-ssh`（L2TP/IPsec専用のPython CLI）である。中心の発想
——「SSHのプロトコル・鍵・known_hosts・agentはホストに残し、**対象へのTCPソケットだけ**を
VPNの名前空間で作る」——をsshcへ持ち込む。

## 成立する範囲

- Dockerがホストで動いている場合だけ使える。無い環境では機能を隠さず、理由を出して断る。
- 1つのVPNプロファイルにつき、接続先は1つ（`host:port`）である。VPNの向こうのネットワーク
  全体を引き込まない。経路とパケットフィルタが対象1つで閉じるので、ホストへの影響と
  取り違えの余地が最小になる。
- 最初はLinuxのみ。macOS・WindowsのDocker Desktopは、backendごとにVM側カーネルの可否が
  変わるため、対応は別に判断する。
- 複数人で共有する仕組みは作らない。

## 経路

```text
engine（ホスト）
   │ net.Dial("unix", …)   ← 0700のディレクトリにあるソケット。TCPポートは開かない
   ▼
中継（VPNコンテナ内）
   │ 対象へのTCP接続
   ▼
トンネルI/F ─ VPN ─ Dockerのbridge ─ VPNサーバー
```

SSHの握手・認証・ホスト鍵の照合・agent転送はすべてホストのengineで行う。コンテナへ渡す
のは「どこへTCPを張るか」だけであり、SSH秘密鍵もVaultの鍵も渡さない。

`docker exec`を使わない理由は二つある。全バイトがdockerdを経由すること、そしてengineが
Goで書かれていてUnixソケットをそのまま`net.Conn`として扱えることである。

TCPポートを公開しない理由は、公開するとホストの他の利用者が全員そのVPN経路に乗れる
ためである。ソケットは利用者だけが読める0700のディレクトリに置く。

## 接ぎ木する場所

`internal/sshclient`の`Dialer.open`が、この接続の輸送を決める唯一の場所である。

```go
if target.ProxyCommand != "" { … }              // 既存
if through != nil { … }                         // 踏み台の上のホップ
if d.Dial != nil { … }
return (&net.Dialer{}).DialContext(…)
```

ここへ`target.VPN`の分岐を足す。制約は`ProxyCommand`と同じで、**踏み台の向こうでは使えない**
（`through != nil`なら拒否する）。VPNはこの機械から出る最初のホップにしか効かない。2ホップ目
以降はSSHトンネルの中を通るので、VPNを通す余地がない。

`Target.VPN`は`internal/app/ssh.go`の`sshParts.target`で載せる。すでに`target.Encoding`を
metadataから載せている場所であり、同じ並びに置く。これによりTerminal・SFTP・CLI・再接続が
同じ経路を通る。Dialerが1つしかないためである。

## 保存する場所

| もの | 置き場 | 理由 |
|---|---|---|
| プロファイル（名前・backend・サーバー・接続先・非秘密の設定） | `metadata.json` | sshc固有の概念であり、同期とバックアップに載る |
| VPNの秘密（PSK、パスワード、秘密鍵） | Vault | 既存の保管庫。sync時も暗号化されたまま運ばれる |
| 接続→プロファイルの紐付け | `metadata.json`の`HostMetadata` | `~/.ssh/config`はOpenSSHが解釈できる語しか書かない |

Vaultの`Kind`に`vpn`を足し、**1プロファイル＝1レコード**とする。値はそのbackendの秘密を
まとめたJSONである。秘密は同時に作られ、同時に回転し、同時に消えるので、レコードを分けると
プロファイルの生成・改名・削除のたびに複数レコードの整合を取ることになる。

`~/.ssh/config`へ書かない帰結として、ホストの`ssh`・`scp`・`git`からはこの経路を使えない。
必要になったら`ProxyCommand`の表記を出力する補助を足す。sshcが利用者の設定ファイルを
勝手に書かない方針は変えない。

## コンテナ

- プロファイル1つにつきコンテナ1つ。`--cap-add NET_ADMIN`、backendが要求するデバイスだけを
  渡す。`--privileged`と`--network host`は使わない。`no-new-privileges`を付ける。
- 秘密はargv・環境変数・イメージ・bind mountに置かず、起動後に標準入力でtmpfsへ渡す。
- fail closed。対象宛のパケットがトンネルI/F以外から出ようとしたら拒否する。トンネルが
  落ちている間、対象への通信がDockerの通常回線へ流れることはない。
- labelで自分のコンテナを識別し、engineの再起動後に回収する。

### イメージと中継

イメージは依存物（`iproute2`、`iptables`、backendのツール、`socat`）だけを含み、初回利用時に
ホストでビルドする。Dockerfileと起動スクリプトはsshcのバイナリへ埋め込む。リリース工程に
手を入れずに始められるためである。配布イメージ（digest固定）へ移すかは、初回ビルドの遅さが
実際に問題になってから判断する。

中継は当面`socat`のUNIXソケット待ち受けとする。独自のGo製中継は、ホストでGoを使えない
以上「ビルダー段を持つイメージ」か「クロスビルドした実行ファイルの埋め込み」を要求する。
L2TP/IPsecの起動手順を入れる段で、その費用を払うかを判断する。

## backend

共通部分は「トンネルI/Fが上がったら、対象への経路を張り、対象宛の他経路を拒否し、中継を
立てる」であり、backendごとに違うのはトンネルの張り方と必要なデバイスだけである。

| backend | デバイス | 秘密 |
|---|---|---|
| `wireguard` | `/dev/net/tun`（userspace実装） | 秘密鍵 |
| `l2tp_ipsec` | `/dev/ppp` | VPNパスワード、IPsec PSK |

最初に`wireguard`を実装する。必要なデバイスが1つで、認証が鍵だけで、**テスト用のサーバーを
コンテナで立てられる**ため、経路・fail closed・ライフサイクルの骨をテストで固められる。
`l2tp_ipsec`（TAINS）はその骨の上に足す。

## ライフサイクル

- 接続が要求した時点で起動し、通信できるまで待つ。進捗は既存の接続ログへ流す。
- そのプロファイルを使うセッションが無くなってから一定時間で停止する。
- トンネルが切れたら中継は新しい接続を断り、既存のTCPは切れる。sshcの既存の再接続が
  走り、そこでVPNも張り直す。
- Dockerが無い、socketに触れない、デバイスが無い場合は、迂回せずに理由を返す。

## 段階

1. `internal/vpn`（プロファイル、docker、中継、readiness、fail closed、wireguard）と
   `Dialer.open`の分岐。CLIから使えるところまで。
2. metadataとVaultへの保存、`sshc vpn`のCLI、HTTP API、Web UI。
3. `l2tp_ipsec` backend。
4. ホストの`ssh`から使うための`ProxyCommand`出力（必要なら）。

## スコープ外

- ホスト全体をVPNへ入れること。
- VPNの向こうのネットワーク全体を引き込むこと（対象は1つ）。
- 複数人での共有、常駐・自動再接続の常時稼働。
- VPN内DNS。対象は`host:port`で指定する。名前で指定したい要求が出たら、コンテナ内での
  解決として別に設計する。
