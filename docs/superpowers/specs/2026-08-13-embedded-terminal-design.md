# 埋め込みターミナル

## 目的

接続を押したとき、外部の端末アプリケーションを起こすのではなく、この
アプリケーション自身の画面の中で端末を開く。利用者のログインシェル（既定で zsh）も
同じ仕組みで開けるようにし、開いているセッションはサイドパネルの一覧から行き来する。

これは macOS の Terminal 起動経路を**置き換える**。Linux には端末起動が存在しない
ため、この仕様が初めてそれを与える。

## この spec の範囲

この会話では 4 つのサブプロジェクトに分割し、この spec は **A のみ**を扱う。

| | サブプロジェクト | 依存 |
|---|---|---|
| **A** | 埋め込みターミナル（この spec）。PTY、WebSocket、xterm.js、ローカルシェル、サイドパネルの一覧と切替 | なし |
| **B** | 埋め込み SSH バックエンド。`x/crypto/ssh` と設定解決器の権威昇格 | なし |
| **C** | Windows 移植。ConPTY、ACL、askpass 代替 | A |
| **D** | Android 外殻。gomobile と WebView、アプリ専有ディレクトリ | B |

A が起動するのは**システムの `ssh` バイナリ**である。`ssh -G` に権威を委ねる現在の
設計も、`SSH_ASKPASS` と一度限りのトークンによる保存済みパスワードの経路も変えない。
B はこの spec の前提でも結果でもない。

## 決定事項

この会話で確定したもの。理由まで残すのは、あとから読む人がひっくり返してよい判断か
どうかを判断できるようにするためである。

**1. PTY は常駐プロセス側で存続する。** ブラウザのタブを閉じてもリロードしても
セッションは生きている。終わるのは、子プロセスが終了したとき、人が閉じたとき、
アプリケーションが終了したときだけである。sshc 本体はログイン項目にもなる常駐
プロセスなので、これが自然な置き場所になる。

**2. 外部ターミナル起動は削除する。** 「端末の選択」に 1 項目足すのではなく、
`internal/platform/macos/terminal.go` の profiles 表・AppleScript・`open -b` 経路と、
`TerminalChoice` まわりを丸ごと取り除く。macOS の利用者は、自分のフォント・tmux・
キーバインドを持つ本物の端末を失う。それを承知の上での決定である。

**3. 起動に action token を要求しない。** vault ゲート（マスターパスワード）だけを
条件とする。これは「新しいゲートを作らない」ではなく、**現在の Terminal 起動にある
ゲートを埋め込み版では外す**という選択である。侵害されたタブは、マスターパスワードが
入っている間、確認なしに任意個のシェルを開ける。

**4. サイドパネルは、接続セクションを上に固定し、その下だけを切り替える。**

**5. 通信路は WebSocket。**

**6. `localhost` はローカルシェルであって ssh 接続ではない。** Home の接続一覧には
出さない。あの一覧は `~/.ssh/config` の投影であり、localhost はそこに存在しない。
入口はサイドパネルの「＋ ローカルシェル」だけである。

## アーキテクチャ

```
ブラウザ                          sshc 本体（常駐）
┌──────────────┐                 ┌────────────────────────────┐
│ xterm.js     │ ── WebSocket ── │ internal/terminal          │
│ (@xterm/*)   │   binary: 生I/O │   Registry                 │
│              │   text:   制御  │   Session ─ PTY ─ ssh/zsh  │
└──────────────┘                 │            └ RingBuffer   │
                                 └────────────────────────────┘
```

新規パッケージ `internal/terminal` が、セッションのレジストリと PTY のライフサイクル
だけを持つ。ssh のコマンドラインの組み立ては既存の `internal/platform` に残し、この
パッケージは「与えられた argv と環境で PTY を起こし、読み書きし、後始末する」以上の
ことを知らない。

**依存の追加は 3 つ。**

- `github.com/creack/pty` — PTY 確保。stdlib に手段がなく、`golang.org/x/term` は
  既存 fd の raw モードとサイズしか扱えない。darwin と linux の両方を覆う
- `github.com/coder/websocket` — 依存を持たない小さな実装（**要確認**）
- npm: `@xterm/xterm` と `@xterm/addon-fit` — VT エミュレータを自前で書くのは論外。
  zsh の zle は alt-screen、bracketed paste、カーソル位置指定を使う

### セッション

```go
type Kind string // "ssh" | "shell"

type Session struct {
    ID       string    // 乱数。alias ではない。同じ alias に複数本開ける
    Kind     Kind
    Alias    string    // Kind == "ssh" のときだけ
    Title    string    // 一覧に出す名前。ssh は alias、shell はシェルの basename
    Started  time.Time
    Exited   *ExitInfo // nil なら生きている
}
```

終了したセッションは、最後の出力を読めるように一覧へ残す。生存上限には数えない。
残す数は 20 本を上限とし、超えたら古いものから捨てる。

### スクロールバック

セッションごとにリングバッファをひとつ持つ。**メモリのみ。ディスクへは一切書かない。**
端末の出力には、保存済みパスワードを使った接続の痕跡も、リモート側が表示した何もかもが
混ざる。世代バックアップも history もこれを受け取らない。

再アタッチ時は、バッファの内容を先に送り、その後ライブの出力へ継ぐ。

## 通信路

### なぜ WebSocket が `/api/` の外にあるか

**ブラウザは WebSocket のハンドシェイクにカスタムヘッダを付けられない。** 現在の
`Security` middleware は `/api/` 配下の要求すべてに `X-SSHC-CSRF` を要求するため、
`/api/` の下に置いたアップグレードは必ず弾かれる。

このリポジトリには既に同じ形の答えがある。`/cli/connect` と askpass のエンドポイントは
「ブラウザ向けの面ではないので `/api/` の外に置き、別の秘密で認証する」という理由で
`/api/` の外にある。同じ規則を使う。

1. `POST /api/v1/terminal/sessions` — 通常の CSRF 付き要求。セッションを作り、
   一度限りの `streamTicket` を返す
2. `GET /terminal/stream?ticket=<...>` — WebSocket アップグレード。`/api/` の外。
   ticket は使い捨てで、ひとつのセッション ID に束縛され、有効期間は 10 秒

アップグレード時にも `Origin` の完全一致は確認する（ブラウザは WebSocket でも
`Origin` を送る）。`Sec-Fetch-Site` が WebSocket ハンドシェイクに付くかは実装時に
**実測して確かめる**。付かないなら、この経路では要求しない。

CSP の `connect-src 'self'` は同一オリジンの `ws://` を許すので、ポリシーの変更は
不要である（**実測して確かめる**）。

### フレーム

- **バイナリフレーム** — PTY の生バイト列。サーバー→クライアントは出力、
  クライアント→サーバーは打鍵。base64 を挟まない
- **テキストフレーム** — JSON の制御メッセージ

```json
クライアント→サーバー  {"resize": {"cols": 120, "rows": 34}}
サーバー→クライアント  {"exit": {"code": 0, "signal": ""}}
```

`resize` は `TIOCSWINSZ` を発行する。これが無いと、全画面を使うプログラム
（`vim`、`top`、`less`）が壊れた幅で描画する。

## API

### 追加

| メソッド | パス | 内容 |
|---|---|---|
| `GET` | `/api/v1/terminal/sessions` | 一覧。生存と終了済みの両方 |
| `POST` | `/api/v1/terminal/sessions` | 開く。body は `{"kind":"ssh","alias":"..."}` または `{"kind":"shell"}`。応答に `id` と `streamTicket` |
| `DELETE` | `/api/v1/terminal/sessions/{id}` | 閉じる。生存中なら子プロセスに SIGHUP、終了済みなら一覧から消す |
| `GET` | `/terminal/stream?ticket=` | WebSocket。`/api/` の外 |

`api/openapi.yaml` は WebSocket のルートを記述しない。契約テスト
（`internal/acceptance/transport_test.go` の `TestRouteTableMatchesTheOpenAPIContract`）は
`f.apiRoutes()` を列挙して突き合わせるので、`/cli/connect` と同じくこのルートも
対象外になる。

### 削除

- `GET /api/v1/terminal/options`
- `GET|POST /api/v1/terminal/preference`
- `POST /api/v1/terminal/launch`

`GET /api/v1/terminal/command` は**残す**。別の端末へ貼るための文字列は、埋め込み
ターミナルがあっても要る場面がある。ただしこのハンドラーが `launchable` の判定と
同じ経路を共有していないかを確認し、共有しているなら切り離すこと。

これに伴い削除するもの: `internal/platform/macos/terminal.go`、`platform.TerminalChoice`
とその検証、`application.Service.SetPreferredTerminal`、`metadata.json` の端末設定、
`DiagnosticsHandlers.SetPreferredTerminal` の配線、diagnostics の `launchable` 報告。

## サイドパネル

上に**接続セクション**を固定し、その下だけをセグメントで切り替える。

```
┌─────────────────────────┐
│ bastion                 │  ヘッダ
├─────────────────────────┤
│ ┌─────────────────────┐ │
│ │ bastion             │ │  接続セクション（常に見える）
│ │ ops@203.0.113.10:22 │ │
│ └─────────────────────┘ │
│ ┌────────┬────────────┐ │
│ │コンソール│    設定    │ │  セグメント
│ └────────┴────────────┘ │
│ ● bastion      ssh    × │
│ ● web-01       ssh    × │  コンソール一覧
│ ● localhost    zsh    × │
│ ○ db-primary   終了   × │
│ ＋ ローカルシェル        │
└─────────────────────────┘
```

- 緑の点は生きているセッション、灰は終了済み。**色は状態にだけ使う**という既存の
  規則の範囲に収まる
- 行を選ぶとそのセッションが主画面に出る
- `×` はそのセッションを閉じる
- 「設定」側は、いまインスペクタが持っている接続の表示設定（色・タグ・お気に入り・
  表示順）をそのまま置く。新しい設定画面を作るのではない

インスペクタの `InspectorContent` は現在 `{label, attention, body}` の 1 面しか持たない。
2 面を持てるように広げる必要がある。**このとき、パネルを持たないセクションが
トグルすら出さないという既存の規則を壊さないこと**（`web/src/ui/Inspector.tsx` の
コメントがその理由を説明している）。

## 設定

`~/.ssh/sshc/metadata.json` に置く。スキーマ版の仕組みと、「知らない値が書かれていても
metadata 全体を読めなくはせず、既定へ戻す」という既存の規則にそのまま乗る。

| 設定 | 既定 | 範囲 |
|---|---|---|
| 同時セッション数の上限 | 50 | 1–200 |
| スクロールバック（セッションあたり） | 256 KiB | 16 KiB–4 MiB |

上限に達した状態で開こうとした要求は `terminal_session_limit` として拒否する。
黙って古いセッションを閉じることはしない。

## 配色

ANSI の 16 色は、生の hex を書くのではなく `index.css` に `--ui-term-*` として
テーマごとに定義する。これにより `web/src/ui/palette.test.ts` の検査に例外
（`palette-exempt`）を足さずに済み、ライトとダークで別の値を与えられる。

## エラー処理

- **子プロセスが即座に終了した場合**、セッションは終了済みとして一覧に残り、出力
  （ssh のエラーメッセージ）が読める状態を保つ。これが「接続できなかった理由」を
  読む唯一の場所になる
- **PTY を確保できない場合**、セッションを作らずに理由を返す
- **WebSocket が切れた場合**、セッションは死なない。同じ ID へ新しい ticket で
  繋ぎ直せる
- **ticket が無効・期限切れ・使用済み**の場合、アップグレードせずに 403 を返す
- **書き込みが詰まった場合**（読まないクライアント）、リングバッファは上書きを
  続け、WebSocket 側は落とす。PTY は止めない

## テスト

**Go**

- レジストリ: 上限の強制、終了済みの保持数、ID の一意性
- リングバッファ: 折り返し、再アタッチ時の再生内容
- ticket: 一度しか使えないこと、別セッションの ID では通らないこと、期限切れ
- PTY はインターフェース越しに偽物へ差し替え、実プロセスなしで上を検査する
- 実 PTY を使う検査は 1 本だけ置き、`/bin/echo` 相当を起こして出力が届くことを見る

**e2e**

- ローカルシェルを開き、`echo` を打ち、出力が画面に出ること
- ページをリロードし、同じセッションが一覧に残り、スクロールバックが再生されること
- 上限まで開き、次が拒否されること

e2e は一時 HOME で実バイナリを動かすので、ローカルシェルの起動はその中で完結する。

**秘密の検査**

`~/.ssh/sshc/` 配下にスクロールバックの内容が現れないことを検査する。既存の漏洩検査は
注入した logger だけを見ているので、これは別に書く必要がある。

## 未解決

- **xterm.js と `require-trusted-types-for 'script'` の相性は未確認。** CSP を緩める
  前に、まず実測すること。緩めるなら README のセキュリティ境界に理由を書く
- Windows の ConPTY は C の範囲
- 埋め込み SSH バックエンドは B の範囲

## README の書き換え

「SSH 実行の境界」の節から、端末の選択・AppleScript・バンドル ID の表・`open -n -b`・
kitty と Ghostty の引数・iTerm2 の `activate` の話が消える。代わりに、埋め込み
ターミナルが何であり、**ブラウザ側に任意コード実行が生まれること**と、それに
action token を要求しないという判断を書く。「更新の境界」で自己更新を落とした理由を
書いたのと同じ書き方で、失ったものと得たものを両方書くこと。

## 実装者への注意

- **このリポジトリは別のセッションが同時に編集していることがある。** コミット前に
  必ず `git diff --cached` で staged 内容そのものを読む。`git status` や
  `git diff --stat` は add の瞬間を保証しない
- `web/src` を変更したコミットには、`make build` で再生成した `internal/ui/dist` を
  必ず同梱する。CI の End to end ジョブが差分を検出して落ちる
- ゲートは `make test`、`make verify-generated`、`make e2e`
- **Linux は `GOOS=linux go vet` では確かめられない。** コンパイルが通ることしか
  分からず、それで実際の退行を 2 度見逃している。Docker で実行すること。root で
  走らせるとパーミッションを見るテストが、素の `--user $(id -u)` では
  `getpwuid` が失敗して `internal/effective` が落ちるので、合成 passwd を渡す:

  ```sh
  printf 'root:x:0:0:root:/root:/bin/bash\nci:x:%s:%s:ci:/tmp:/bin/sh\n' "$(id -u)" "$(id -g)" > /tmp/passwd
  printf 'root:x:0:\nci:x:%s:\n' "$(id -g)" > /tmp/group
  docker run --rm --user "$(id -u):$(id -g)" \
    -v /tmp/passwd:/etc/passwd:ro -v /tmp/group:/etc/group:ro \
    -e HOME=/tmp -e GOCACHE=/tmp/gocache -e GOMODCACHE=/tmp/gomod \
    -v "$PWD":/src -w /src golang:1.26 \
    sh -c "go build ./... && go vet ./... && go test ./... -count=1"
  ```

- `make e2e` はホスト向けにビルドするので、この Mac では darwin のバイナリを
  駆動する。プラットフォーム依存の挙動を e2e で表明するなら `process.platform` で
  分岐しないと、CI で緑・ローカルで赤になる
- 新しい文言は `web/src/i18n` の en と ja の両方へ入れる。en が正で、欠落は
  コンパイルエラーになるが、余りは検出されない
