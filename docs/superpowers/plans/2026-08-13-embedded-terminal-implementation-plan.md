# Embedded Terminal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 接続を押したとき、外部の端末アプリケーションを起こすのではなく、このアプリケーション自身の画面の中で端末を開く。ログインシェルも同じ仕組みで開き、開いているセッションはサイドパネルの一覧から行き来する。macOS の Terminal 起動経路を**置き換え**、Linux には初めて端末を与える。

**Architecture:** 常駐プロセスが PTY を所有する。新規パッケージ `internal/terminal` がセッションのレジストリと PTY のライフサイクルだけを持ち、ssh の argv 組み立ては `internal/platform` に残る。ブラウザとは `/api/` の外にある WebSocket 一本で繋ぎ、バイナリフレームを生 I/O、テキストフレームを制御に使う。VT エミュレーションは xterm.js。

**Tech Stack:** Go 1.26 / echo v5 / `github.com/creack/pty` / `github.com/coder/websocket` / React 19 / `@xterm/xterm` 6.0.0 / `@xterm/addon-fit` 0.11.0

---

## Global Constraints

- **サブプロジェクト A のみ。** 起動するのはシステムの `ssh` バイナリである。`ssh -G` に権威を委ねる設計も、`SSH_ASKPASS` と一度限りのトークンによる保存済み鍵パスフレーズの経路も変えない。埋め込み SSH バックエンド（B）、Windows（C）、Android（D）はこの計画の外。
- **スクロールバックはメモリのみ。** ディスクへ一切書かない。世代バックアップも history も remote sync もこれを受け取らない。
- **起動に action token を要求しない。** vault ゲート（マスターパスワード）だけを条件とする。
- **`web/src` を変えたコミットには再生成した `internal/ui/dist` を同梱する。**
- ゲートは `make test`、`make verify-generated`、`make e2e`。Linux は Docker で実行して確かめる（`GOOS=linux go vet` では確かめられない）。
- 新しい文言は `web/src/i18n/messages/en.ts` と `ja.ts` の両方へ入れる。

---

## 設計判断

理由まで残すのは、あとから読む人が「ひっくり返してよい判断か」を判断できるようにするためである。

### D1. `internal/terminal` の継ぎ目は `Starter` ひとつ

`Registry`（一覧と上限）、`Session`（PTY 一本と、そこにぶら下がるアタッチ）、`Ring`（スクロールバック）、`Tickets`（使い捨ての認可）。PTY を確保する `Starter` と、確保された一本を表す `Process` だけがインターフェースで、テストはここを偽物へ差し替えて実プロセスなしにレジストリの上を検査する。実 PTY を使う検査は 1 本だけ置く。

`creack/pty` を使うのは stdlib に手段が無いからである。`golang.org/x/term` が扱えるのは既存 fd の raw モードとサイズだけで、対を確保できない。

### D2. ssh は PTY の中で直接起こす。`sshc <alias>` を経由しない

`sshc <alias>` を PTY で起こす案を採らない理由は 2 つある。

1. **SIGHUP が届く先が違う。** `cmd/sshc` の `executeSSH` は `exec.Command(...).Run()` であって exec 置換ではない。PTY の子は `sshc` になり、閉じる操作はそれを殺すが、孫の `ssh` は親を失って残る。直接 `ssh` を起こせば、子は session leader なのでプロセスグループ全体に SIGHUP が届く。
2. **自分自身への HTTP 往復になる。** `sshc <alias>` は handoff ファイルを読んで `/cli/connect` を叩く。同じプロセスの中に答えがあるのに。

代わりに、いま `cmd/sshc/connect.go` に閉じている 3 つ — `connectArguments`、`connectEnvironmentForCredential`、`createConnectionConfig` — を `internal/platform` へ引き上げ、**CLI と埋め込みターミナルの両方がそれを呼ぶ**。二つ目の実装は作らない。凍結した ssh 設定の後始末は、セッション終了時に一度だけ走る `Spec.Cleanup` が持つ。

### D3. metadata は新しいキーを使い、スキーマ版を 3 へ上げる

v2 の文書は `"terminal": "iterm2"` という**文字列**を持つ。同じキーをオブジェクトへ変えると `json.Unmarshal` は文書全体で失敗し、グループも色もお気に入りも道連れに読めなくなる。これは「知らない値が書かれていても metadata 全体を読めなくはせず、既定へ戻す」という既存の規則を正面から破る。

したがって `Terminal` と `CustomTerminal` は**フィールドごと削除**し（v1→v2 でグループメンバーシップを落としたのと同じ手口。`json.Unmarshal` は構造体にもう存在しないものを無視する）、新しい `embeddedTerminal` を足す。

`MetadataSchemaVersion` を 3 へ上げるのは、古いビルドがこの文書を読んで新しい設定を黙って落とし、次の保存で消してしまうのを防ぐためである。それがこの番号の役目そのものだ。

| 設定 | JSON | 既定 | 範囲 |
|---|---|---|---|
| 同時セッション数の上限 | `embeddedTerminal.maxSessions` | 50 | 1–200 |
| スクロールバック（セッションあたり） | `embeddedTerminal.scrollbackBytes` | 262144 | 16384–4194304 |

範囲の外は拒否ではなく既定へ戻す（読み取り側だから）。画面は作らない。仕様が求めていない。

### D4. WebSocket は `/api/` の外。CSP は緩めない

ブラウザは WebSocket のハンドシェイクにカスタムヘッダを付けられず、`Security` middleware は `/api/` 配下のすべてに `X-SSHC-CSRF` を要求するので、`/api/` の下に置いたアップグレードは必ず弾かれる。`/cli/connect` と askpass が `/api/` の外にあるのと同じ規則を使う。

- `POST /api/v1/terminal/sessions`（通常の CSRF 付き要求）が一度限りの `streamTicket` を返す。
- `GET /terminal/stream?ticket=…` がアップグレードする。ticket は使い捨て、ひとつのセッション ID に束縛、有効期間 10 秒。
- アップグレード時も `Origin` の完全一致を**必ず**確認する。
- `Sec-Fetch-Site` は**実測してから決める**。到達したヘッダを記録する Go テストを 1 本置き、Chromium が送るなら必須にし、送らないならこの経路では要求しない。

**CSP は `style-src` だけを緩めた（実測の結果）。** `@xterm/xterm` の配布物 5.5.0 と 6.0.0 の両方を展開して数えた結果、`innerHTML` / `outerHTML` / `insertAdjacentHTML` / `document.write` / `new Function` / `eval(` / `trustedTypes` / `new Worker` / `createObjectURL` / `srcdoc` の出現は**すべて 0 件**である。DOM は `createElement` と `textContent` で組み立てられている。したがって `require-trusted-types-for 'script'` も `script-src 'self'` も緩めていない。

**ただし `style-src` は緩めることになった。** これは事前の想定になかった。xterm.js は文字の実寸を測ってから、その寸法を持つ規則を `<style>` 要素として差し込み、DOM レンダラーは各セルへ `setAttribute("style", …)` を書く。nonce を渡す口は無い（`ITerminalOptions` にそのフィールドが無い）。実際のブラウザで走らせると `style-src 'self'` が両方を拒否し、20 件の違反が出た。したがって `style-src 'self' 'unsafe-inline'` にした。緩めたのはここだけで、`internal/acceptance/transport_test.go` が「`unsafe-inline` はちょうど 1 個で、script-src には現れない」ことを表明している。

`connect-src 'self'` が同一オリジンの `ws://` を許すことは実測で確認した（変更不要）。`Sec-Fetch-Site` は Chromium が `same-origin` を送ることを確認したが、送らないブラウザで端末が開けなくなるのは割に合わないので、**付いていれば検査し、付いていなければチケットと Origin だけで判断する**形にしてある。

`internal/acceptance/transport_test.go` の 2 本（トランスポート掃引と OpenAPI 契約）はどちらも `f.apiRoutes()`（`/api/` 前方一致）を列挙するので、`/terminal/stream` は `/cli/connect` と同じく自動的に対象外になる。**新しく足す `/api/v1/terminal/sessions` は掃引の対象になる**ので、`{}` を投げたときにトランスポート由来でない 400 を返すことを確かめる。

### D5. `InspectorContent` は「固定の頭」と「切り替わる面」に分ける

```ts
type InspectorContent = {
  label: string;
  attention: boolean;
  header?: ReactNode;                                  // 接続セクション（常に見える）
  panes: { key: string; label: string; body: ReactNode }[];
} | null;
```

`null` を返す道は残るので、**パネルを持たないセクションがトグルすら出さないという既存の規則は壊れない**（`web/src/ui/Inspector.tsx` のコメントがその理由を説明している）。`panes` が 1 枚のときはセグメントを描かないので、Groups の見た目は今と 1 ピクセルも変わらない。

Connections が差し出すもの:

- `header` — alias と `ops@203.0.113.10:22`。既存の `summarizeConnection()`（`connectionSavedState.ts`）が両方を既に返す。接続が開いていなければ省く。
- `panes` — 「コンソール」（常にある。セッション一覧と「＋ ローカルシェル」）と「設定」（接続が開いているときだけ。いまの `HostInspector` をそのまま置く）。

### D6. 端末は主画面に出る。URL には載せない

コンソールの行を選ぶと、Connections の右カラムが接続詳細ではなく端末になる。共有可能な URL に載せる価値のある状態ではない（他人のマシンで開いても、そのセッションは存在しない）。リロード後もセッションが一覧に残ることは仕様の要求で、それはサーバー側の状態なので URL とは無関係に成り立つ。

Home の接続ボタンは、埋め込みセッションを開いてから Connections へ移動し、それを選択状態にする。

### D7. ANSI 16 色は `index.css` のトークン

`--ui-term-*` をテーマごとに定義し、`getComputedStyle` で読んで `new Terminal({ theme })` へ渡す。`useTheme().resolved` が変わったら読み直して `term.options.theme` を差し替える。生の hex をコンポーネントに書かないので、`web/src/ui/palette.test.ts` に `palette-exempt` を足さずに済む。

トークン: `--ui-term-bg` / `-fg` / `-cursor` / `-selection` と、`-black` `-red` `-green` `-yellow` `-blue` `-magenta` `-cyan` `-white` の通常と `-bright-*`（計 20）。

### D8. 削除するもの

| 削除 | 理由 |
|---|---|
| `internal/platform/macos/terminal.go` と `_test.go` | profiles 表・AppleScript・`open -n -b` 経路 |
| `platform.TerminalChoice`・`TerminalID`・在庫・4 つの Launcher インターフェース | 起動先の選択が無くなる |
| `application.Service.SetPreferredTerminal` / `PreferredTerminal` | 保存する選択が無い |
| `Metadata.Terminal` / `CustomTerminal` / `ErrMetadataTerminal` | D3 |
| `diagnostics` の `LaunchTerminal` / `LaunchTerminalWithPassword` / `TerminalOptions` / `selectedTerminal` / `launchable` | 起動しない |
| `GET /api/v1/terminal/options`、`GET|PUT /api/v1/terminal/preference`、`POST /api/v1/terminal/launch` | 同上 |
| `session.ActionTerminalLaunch` | 起動に確認を要求しない（決定 3） |
| `web/src/settings/TerminalPreferenceSection.tsx` と `_test.tsx` | 選ぶものが無い |
| `internal/acceptance/injection_darwin_test.go` | AppleScript が無くなる |

`GET /api/v1/terminal/command` は**残す**。別の端末へ貼るための文字列は要る場面がある。ただし応答から `launchable` を落とす（起動可否という概念が消えるため）。`warning` は残す — 安全でない alias の説明は依然として必要である。

---

## Task 1: `internal/terminal` — レジストリ、リングバッファ、チケット

**Files:**
- Create: `internal/terminal/terminal.go`（`Kind`、`Size`、`ExitInfo`、`Command`、`Process`、`Starter`、`Limits`、エラー）
- Create: `internal/terminal/ring.go`, `internal/terminal/session.go`, `internal/terminal/registry.go`, `internal/terminal/ticket.go`
- Create: `internal/terminal/pty_unix.go`（`//go:build unix`、`creack/pty`）
- Create: `internal/terminal/{ring,registry,ticket,pty}_test.go`
- Modify: `go.mod`, `go.sum`

> 作業ツリーにこの Task の下書きが既にある（`internal/terminal/`、`go.mod` の `creack/pty`）。レビュー後にそこから継ぐ。

**Interfaces:**
- Produces: `Registry.Open(Spec) (*Session, error)`、`Sessions() []View`、`Lookup`、`Close(id)`、`Shutdown()`。`Session.Attach() ([]byte, *Stream)`、`Write`、`Resize`、`Hangup`、`Exit`。`Tickets.Issue(sessionID)`、`Redeem(token) (sessionID, bool)`。
- Consumes: `Starter`（本番は `UnixStarter`、テストは偽物）。

- [x] **Step 1: 失敗するテストを書く**

リングバッファ（折り返し、上限ちょうど、上限より長い単発書き込み、スナップショットが複製であること）。レジストリ（生存上限の強制、終了済みは上限に数えない、終了済みの保持は 20 本で古い順に捨てる、ID の一意性、`Attach` がバッファを再生してからライブへ継ぐ、読まないアタッチは落とされるが PTY は止まらない）。チケット（一度しか使えない、束縛されたセッション ID しか返さない、期限切れ）。

- [x] **Step 2: RED を確認する** — Run: `go test ./internal/terminal/`

- [x] **Step 3: 実装する**

- [x] **Step 4: 実 PTY の検査を 1 本足す** — `/bin/echo` 相当を起こし、出力がリングバッファへ届き、`Wait` が終了理由を返すこと。

- [x] **Step 5: GREEN を確認する** — Run: `go test ./internal/terminal/ -count=1 && go test -race ./internal/terminal/`

---

## Task 2: ssh の起動一式を `internal/platform` へ引き上げる

**Files:**
- Create: `internal/platform/interactive.go`, `internal/platform/interactive_test.go`
- Modify: `cmd/sshc/connect.go`（引き上げた関数を呼ぶだけにする）, `cmd/sshc/connect_test.go`

**Interfaces:**
- Produces: `platform.InteractiveSSH(ssh string, alias string, credential AskpassCredential) (Command, cleanup func(), error)` — argv と、`SSH_ASKPASS` 一式を含む完全な環境と、凍結した設定の後始末を返す。
- Consumes: 既存の `MinimalEnvironment` は使わない（これはユーザー自身の環境を継ぐ接続であり、非対話の検査ではない）。

- [x] **Step 1: 失敗するテストを書く** — 引き上げた関数が、いまの `connectEnvironmentForCredential` と同じ環境（ユーザーの `SSH_ASKPASS` を必ず上書きすること、武装しない接続でも 5 変数を取り除くこと）と、同じ argv（`-F` / `-i` / `IdentitiesOnly=yes`）を返すこと。
- [x] **Step 2: RED を確認する**
- [x] **Step 3: 移動し、`cmd/sshc` を呼び出し側に変える**
- [x] **Step 4: GREEN を確認する** — Run: `go test ./cmd/sshc/ ./internal/platform/... -count=1`

---

## Task 3: metadata の設定と、外部ターミナル設定の削除

**Files:**
- Modify: `internal/application/metadata.go`, `metadata_test.go`, `internal/application/service.go`, `service_test.go`
- Modify: `api/openapi.yaml`（`Metadata` から `terminal`/`customTerminal` を落とし `embeddedTerminal` を足す。`TerminalID`・`CustomTerminal` スキーマを削除）
- Delete: `internal/platform/terminal.go` の `TerminalChoice` 周辺

- [x] **Step 1: 失敗するテストを書く** — v2 の文書（`"terminal": "iterm2"` を含む）が読め、その値を静かに失うこと。範囲外の `maxSessions` / `scrollbackBytes` が既定へ戻ること。v4 の文書が拒否されること。
- [x] **Step 2: RED を確認する**
- [x] **Step 3: 実装し、`MetadataSchemaVersion` を 3 にする**
- [x] **Step 4: GREEN を確認する** — Run: `go test ./internal/application/ -count=1`

---

## Task 4: HTTP API と WebSocket

**Files:**
- Create: `internal/httpserver/terminal.go`, `terminal_test.go`
- Modify: `internal/httpserver/server.go`（`Options.Terminals` / `Options.TerminalTickets` を足し、ルートを登録）
- Modify: `internal/httpserver/diagnostics.go`（`TerminalOptions` / `TerminalPreference` / `TerminalLaunch` を削除、`TerminalCommand` から `launchable` を落とす）
- Modify: `internal/diagnostics/service.go`, `internal/session/action.go`, `internal/app/run.go`
- Modify: `api/openapi.yaml`, `internal/api/models.gen.go`（生成）
- Modify: `go.mod`（`coder/websocket`）

**Interfaces:**

| メソッド | パス | 内容 |
|---|---|---|
| `GET` | `/api/v1/terminal/sessions` | 一覧。生存と終了済みの両方 |
| `POST` | `/api/v1/terminal/sessions` | `{"kind":"ssh","alias":"…","cols":120,"rows":34}` または `{"kind":"shell",…}`。応答に `id` と `streamTicket` |
| `DELETE` | `/api/v1/terminal/sessions/{id}` | 生存中なら SIGHUP、終了済みなら一覧から消す |
| `GET` | `/terminal/stream?ticket=` | WebSocket。`/api/` の外。**openapi.yaml には書かない** |

フレーム: バイナリ = 生 I/O（base64 を挟まない）、テキスト = `{"resize":{"cols":…,"rows":…}}` と `{"exit":{"code":…,"signal":""}}`。

拒否コード: `terminal_session_limit`（409）、`unsafe_alias`（400）、`terminal_start_failed`（500）、ticket 不正は**アップグレードせずに 403**。

- [x] **Step 1: 失敗するテストを書く** — 上限に達した POST が `terminal_session_limit` を返すこと。無効・期限切れ・使用済み ticket が 403 でアップグレードしないこと。`DELETE` が生存中と終了済みで別の意味になること。子プロセスが即座に終了したセッションが一覧に残り、出力が読めること。
- [x] **Step 2: RED を確認する**
- [x] **Step 3: 実装する。** ローカルシェルは `$SHELL` を読み、無ければ `getpwuid` のシェル、それも無ければ `/bin/sh`。
- [x] **Step 4: `Sec-Fetch-Site` を実測する** — 到達したヘッダを記録するテストを置き、Chromium の実測値に従って必須にするかを決め、結果をこの計画と README へ書く。
- [x] **Step 5: 生成物を更新する** — Run: `make verify-generated`
- [x] **Step 6: GREEN を確認する** — Run: `go test ./... -count=1 && go test -race ./...`

---

## Task 5: 外部ターミナル起動の削除

**Files:**
- Delete: `internal/platform/macos/terminal.go`, `terminal_test.go`, `internal/acceptance/injection_darwin_test.go`
- Modify: `cmd/sshc/wiring*.go`, `internal/app/run.go`, `internal/acceptance/{harness,leak,limits,conditions}_test.go`
- Modify: `internal/httpserver/diagnostics_test.go`, `internal/diagnostics/service_test.go`

- [x] **Step 1: 削除し、ハーネスの `recordingTerminal` を取り除く**
- [x] **Step 2: `guardedRoutes` から `/api/v1/terminal/launch` の行を落とす** — 他の行が使う「別の kind のトークン」の相手を `ActionEvaluate` へ差し替える。
- [x] **Step 3: GREEN を確認する** — Run: `go build ./... && go vet ./... && go test ./... -count=1`

---

## Task 6: web — xterm.js、サイドパネル、配色

**Files:**
- Modify: `web/package.json`, `package-lock.json`（`@xterm/xterm` 6.0.0、`@xterm/addon-fit` 0.11.0）
- Create: `web/src/terminal/{TerminalView,ConsoleList,session}.{tsx,ts}` と各 `.test.tsx`
- Modify: `web/src/ui/Inspector.tsx`, `web/src/App.tsx`, `web/src/connections/ConnectionsPage.tsx`, `HostInspector.tsx`, `ConnectionSummary.tsx`, `web/src/overview/OverviewPanel.tsx`
- Modify: `web/src/api/integrations.ts`, `web/src/index.css`, `web/src/i18n/messages/{en,ja}.ts`
- Modify: `web/src/settings/SettingsPanel.tsx` / Delete: `TerminalPreferenceSection.{tsx,test.tsx}`

- [x] **Step 1: 失敗するテストを書く** — `InspectorContent` が 2 面を持てること、1 面のときセグメントを描かないこと、`null` のときトグルが出ないこと。コンソール一覧が生存（緑）と終了済み（灰）を書き分けること。上限拒否が画面に出ること。
- [x] **Step 2: RED を確認する** — Run: `npm test --prefix web`
- [x] **Step 3: 実装する。** `--ui-term-*` を light / dark 両方に定義し、`getComputedStyle` 経由で xterm へ渡す。テーマ変更で読み直す。
- [x] **Step 4: GREEN を確認する** — Run: `npm test --prefix web && npm run typecheck --prefix web && npm run check:i18n --prefix web`

---

## Task 7: e2e、秘密の検査、README

**Files:**
- Create: `web/e2e/terminal.spec.ts`
- Create: `internal/acceptance/scrollback_test.go`
- Modify: `web/e2e/{home,connections,routing}.spec.ts`, `README.md`

- [x] **Step 1: e2e を書く** — ローカルシェルを開いて `echo` を打ち出力が出ること。リロード後も一覧に残りスクロールバックが再生されること。上限まで開いて次が拒否されること。CSP 違反が 1 件も出ないこと（`page.on("console")` を監視する）。
- [x] **Step 2: 秘密の検査を書く** — `~/.ssh/sshc/` 配下にスクロールバックの内容が現れないこと。既存の漏洩検査は注入した logger だけを見ているので、これは別に書く。
- [x] **Step 3: README を書き換える** — 「SSH 実行の境界」から端末の選択・AppleScript・バンドル ID の表・`open -n -b`・kitty と Ghostty の引数・iTerm2 の `activate` を消す。代わりに、埋め込みターミナルが何であり、**ブラウザ側に任意コード実行が生まれること**と、それに action token を要求しないという判断を書く。「更新の境界」で自己更新を落とした理由を書いたのと同じ書き方で、**失ったものと得たものを両方**書く。CSP を緩めなかったことと、その実測結果も書く。
- [x] **Step 4: 全ゲートを通す** — Run: `make test && make verify-generated && make e2e`
- [x] **Step 5: Linux を Docker で確かめる**

```sh
printf 'root:x:0:0:root:/root:/bin/bash\nci:x:%s:%s:ci:/tmp:/bin/sh\n' "$(id -u)" "$(id -g)" > /tmp/passwd
printf 'root:x:0:\nci:x:%s:\n' "$(id -g)" > /tmp/group
docker run --rm --user "$(id -u):$(id -g)" \
  -v /tmp/passwd:/etc/passwd:ro -v /tmp/group:/etc/group:ro \
  -e HOME=/tmp -e GOCACHE=/tmp/gocache -e GOMODCACHE=/tmp/gomod \
  -v "$PWD":/src -w /src golang:1.26 \
  sh -c "go build ./... && go vet ./... && go test ./... -count=1"
```

---

## 実装中に分かったこと

- **`style-src` は緩めることになった。** 上の D4 に実測の結果を書いた。
- **繋ぎ直すためのルートが要る。** チケットは使い捨てなので、リロードしたページは自分のセッションへ繋ぎ直す手段を持たない。仕様の API 表には無いが、`POST /api/v1/terminal/sessions/{id}/stream` を足した。これが無いと「リロードしてもスクロールバックが再生される」という要求そのものが成立しない。
- **xterm.js は別の chunk に切る必要があった。** 400 kB を接続画面の chunk に入れると、その画面のマウントが遅れ、URL の正規化（`/connections/` → `/connections/servers`）が end-to-end の表明より後になった。`TerminalView` を `lazy` にして、接続画面の chunk は 427 kB から 84 kB に戻した。
- **e2e は OpenSSH を起動しない。** その約束は既存のスイート全体の前提なので、ssh セッションを開く spec は「埋め込みの要求が出たこと」までを見て止める。コンソールが実際に描かれるところは、リモートに触れないローカルシェルの spec が見ている。

## 失うもの

macOS の利用者は、自分のフォント・tmux・キーバインドを持つ本物の端末を失う。承知の上での決定である。

侵害されたタブは、マスターパスワードが入っている間、確認なしに任意個のシェルを開ける。これは新しいゲートを作らなかったのではなく、**いまの Terminal 起動にあるゲートを埋め込み版では外す**という選択である。README にそう書く。
