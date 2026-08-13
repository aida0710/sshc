# 設定解決器を権威にする

## 目的

`~/.ssh/config` を読んで「この alias に接続すると実際に何が使われるか」に答えるのを、
`ssh -G` ではなくこのアプリケーション自身にする。

これは埋め込み SSH バックエンド（サブプロジェクト B）の第一歩である。プロセス内で
SSH を話すなら、接続に使う値を自分で決められなければならない。

## この spec の範囲

**B1 のみ。** 解決器を権威に昇格させ、`ssh -G` を回す経路を消すところまで。
プロセス内 SSH クライアント（B2）、askpass の撤去と CLI の移行（B3）、落とす機能の
確定（B4）は別の spec である。

`~/.ssh/config` は正本のまま残す。自前のデータベースへは移さない。

## なぜ権威を移すのか

`ssh -G` は 3 つの制約を連れてくる。

- **設定を読むだけで `Match exec` が走る。** だから評価に確認ダイアログが要る
- **Android には `ssh` が無い。** 外部の実行ファイルに権威を委ねる設計は、そこで破綻する
- **プロセス内で SSH を話すなら、どのみち値を自分で決める必要がある**

## 決定事項

**1. 既定値を持つのは、sshc が実際に読む値だけ。** 対象は 1 か所に表として書き、
そこに無いものは既定値を持たない。最初の表はこれである。

| キーワード | 既定 | 誰が読むか |
| --- | --- | --- |
| `HostName` | alias そのもの | 接続、画面、到達性 |
| `User` | ローカルの user 名 | 接続、画面、askpass の宛先照合 |
| `Port` | `22` | 接続、画面、到達性 |
| `IdentityFile` | OpenSSH と同じ既定の並び（累積） | 接続、鍵の利用状況 |
| `ProxyJump` | 無し | 接続、到達性の但し書き |

書かれている他のキーワードは「**書かれている値**」としてそのまま返す。既定値は持た
ない。**OpenSSH の既定値表を丸ごと持つことはしない**——あれは版ごとに変わるので、
追い続ける保守が利用者に何も返さない。

表に足すのは、sshc がその値で何かを決めるようになったときだけである。「画面に出す
ため」は理由にならない。書かれている値はもう返せている。

**2. `Match exec` は評価せず、理由を添えて拒む。** これにより**解決器は何も実行
しない**と言い切れる。評価時の確認ダイアログは丸ごと消える。`Match exec` を含む
設定の alias は「解決できない」と答え、接続も断る。`~/.ssh/config` を正本のまま
残すので、その人は端末から `ssh` で繋げる——逃げ道は構造上ある。

**3. `ssh -G` を回す経路は B1 の最後に削除する。** 解決器が差分テストを通るまでは
並走させ、通ったら `POST /api/v1/diagnostics/effective`、確認ダイアログ、
`internal/effective/evaluate.go` を消す。**権威は 1 つでなければならない。**

**4. 2 つある実装を 1 つにまとめる。** いま「この alias は何に解決されるか」に答える
コードが 2 つある。

- `internal/effective.Project()` — 出所の説明用
- `internal/application.ComputeEffective()` — 接続画面用。`Approximate: true` を必ず付ける

どちらも「権威ではない」と言うことで共存している。片方を権威に昇格させ、もう片方を
残すのは、`MatchHostLine` のコメントが警告している状態そのものを権威の位置に作る
ことである。**先に 1 つへ寄せてから昇格させる。**

## いま在るもの

昇格の土台はほぼ揃っている。

| | 状態 |
| --- | --- |
| 読み込み順の走査（`Include` を行の位置で降りる） | `walkLoadOrder` にある |
| Host パターンの照合（ワイルドカード、否定） | `blockApplies` と `effective.MatchPattern` にある |
| 最初の値が勝つ規則 | `Project()` の `claimed` にある |
| 複数値のキーワード（`IdentityFile` など） | `Values.Entries map[string][]string` が既に持てる |
| 実機の OpenSSH との差分テスト | `TestProjectionMatchesInstalledOpenSSH` が既に走る |

## 足りないもの

**A. `Match` ブロックの評価。** いまは値を一切寄与せず、complexity として記録するだけ
である。評価すべき条件は `host`、`originalhost`、`user`、`localuser`、`final`、
`canonical`、`tagged`、`all`。どれもプロセスを起こさずに判定できる。`exec` だけが
例外で、決定事項 2 のとおり拒む。

**B. トークン展開。** `%h`（HostName）、`%p`（Port）、`%r`（リモート user）、
`%u`（ローカル user）、`%d`（ホームディレクトリ）、`%n`（元の alias）、`%L`／`%l`
（ホスト名）、`%C`（ハッシュ）、`%i`（uid）。いまは展開せず
`include_unsupported_expansion` として報告している。

**C. 複数値のキーワードの累積。** `IdentityFile`、`CertificateFile`、`LocalForward`、
`RemoteForward`、`DynamicForward`、`SendEnv`、`SetEnv` は、最初の 1 つが勝つのでは
なく積み上がる。

**この表は既に `internal/application` の `cumulativeKeywords` にあり、
`ComputeEffective` は正しく扱っている。** 誤っているのは `internal/effective.Project()`
の方で、あちらは `claimed` という一律の先勝ちしか持たない。その結果、`IdentityFile`
を 2 行書いた設定では、2 行目の `Winner` が false になって画面へ出る
（`diagnostics.go:143`）——OpenSSH は両方を使うのに。

2 つを 1 つへ寄せるとき、この表を共有の場所へ移す。**同じ問いに答える表が 2 つある
状態を、権威の位置に持ち込まない。**

**D. 既定値。** `Port 22`、`User`＝ローカルの user 名、`IdentityFile` の既定の並び。
決定事項 1 の範囲に限る。

**E. `CanonicalizeHostname`。** 使う人が少なく、再解決のパスを丸ごと足すので、
**B1 では対応しない。** 設定に現れたら「解決できない」として拒む。

## 出力の形

権威になるとは、`Values` を自分で作れるということである。

```go
// Resolve は、この alias に接続したときに実際に使われる値を返す。
//
// 何も実行しない。Match exec を含む設定は、値ではなく理由を返す。
func Resolve(graph *config.Graph, alias string, local LocalFacts) (Values, []Refusal, error)
```

`LocalFacts` はローカルの user 名・ホーム・uid・ホスト名。トークン展開に要る事実で
あり、注入するのは、テストが本物のホームへ届かないようにするためである。

`Refusal` は、`Complexity` と違って**答えを出さない理由**である。`Match exec` と
`CanonicalizeHostname` の 2 つから始まる。complexity は「説明はできるが単純ではない」
という印なので、権威になったあとは意味が変わる——ワイルドカードで一致したことも、
alias が重複していることも、**答えは確定する**。表示のための情報として残すが、
「権威に委ねる」という意味は外す。

## 完成条件

**差分テストがフィクスチャ群に対して `ssh -G` と一致すること。** いまの
`TestProjectionMatchesInstalledOpenSSH` はフィクスチャが設定したキーワードだけを
比較する形なので、そのまま完成条件として使える。フィクスチャを増やしながら進める。

最低限、以下を覆う。

- Host パターン（完全一致、ワイルドカード、否定、大文字小文字）
- 複数ブロックにまたがる最初の値が勝つ規則
- `Include` の位置による読み込み順
- `Match host` / `user` / `originalhost` / `localuser` / `final` / `canonical` / `tagged`
- トークン展開（`%h` `%p` `%r` `%u` `%d` `%n`）
- 累積するキーワード（`IdentityFile` を複数、`SetEnv` を複数）
- 既定値（`Port`、`User`）

**`ssh` が無い環境ではスキップする**（いまと同じ）。CI は ubuntu と macOS の両方で
走るので、両方の OpenSSH に対して確かめられる。

## 進め方

作っている間、**何も壊さない**。

1. `Resolve` を新しい関数として書き、差分テストで育てる。既存の 2 実装はそのまま
2. フィクスチャ群を通ったら、`ComputeEffective` と `Project` の呼び出し側を
   `Resolve` へ寄せる
3. 画面から `Approximate` と `explained_values_only` を外す
4. `ssh -G` の経路（API、確認ダイアログ、`Evaluator`）を削除する

**3 と 4 は別のコミットにする。** 3 は「答えが確定するようになった」という表示の変更、
4 は機能の削除であり、片方だけ戻したくなる可能性が違う。

## エラー処理

| 起きたこと | 何を返すか |
| --- | --- |
| `Match exec` がグラフのどこかにある | `Refusal`。その alias の値は返さない |
| `CanonicalizeHostname` が設定されている | `Refusal` |
| `Include` が解決できない | いまと同じく診断として報告し、読めた範囲で解決する |
| トークンが展開できない（`LocalFacts` が欠ける） | そのキーワードだけ `Refusal` |

**部分的な答えを黙って返さない。** 接続に使う値が 1 つでも確定しないなら、その alias
は解決できていない。

## テスト

- **差分テスト**（上記）が主。実機の OpenSSH と突き合わせる
- **単体テスト** — `Match` の各条件、トークン展開、累積キーワード、既定値
- **fuzz** — `FuzzParseValues` は `ssh -G` の出力パーサー用なので、経路の削除と共に
  役目を終える。代わりに `Resolve` を fuzz する（グラフを与えて panic しないこと）
- 既存の `internal/acceptance` の完成条件（`TestDesignCompletionConditions`）に、
  権威が移ったことを反映する

## README の書き換え

「権威は `ssh -G` に委ねます」「Effective タブと Diagnostics タブは値の出所説明のみ」
「複雑な外部ルール」の各節が変わる。**失ったもの**（OpenSSH 自身の答えを画面から
確かめられなくなること、`Match exec` と `CanonicalizeHostname` を使う設定を解決でき
なくなること）と、**得たもの**（何も実行せずに答えられること、Android でも同じ答えを
出せること）を両方書く。

## 未解決

- **`Match final` の二周目。** OpenSSH は `final` を含む設定を二度読む。B1 の範囲で
  実装するが、差分テストで確かめるまでは危険な部分として扱う
- **アルゴリズムの優先順位** — 鍵交換や暗号の既定は B2（プロセス内 SSH）の話であり、
  ここでは扱わない

## 実装者への注意

- **このリポジトリは別のセッションが同時に編集していることがある。** コミット前に
  必ず `git diff --cached` で staged 内容そのものを読む
- Linux は `GOOS=linux go vet` では確かめられない。Docker で走らせること
- 新しい文言は `web/src/i18n` の en と ja の両方へ入れる
