# プロセス内 SSH クライアント 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 埋め込みターミナルの SSH セッションを、外部の `ssh` ではなくこのプロセスの中で話す。

**Architecture:** 新しい `internal/sshclient` が `golang.org/x/crypto/ssh` を使って接続し、`terminal.Process` を実装する。`terminal.Registry` は SSH を知らないまま、`Spec.Open` という一つの継ぎ目を通してそれを受け取る。接続に要る値は `internal/effective.Resolve` の答えから組み立てる——このパッケージは `~/.ssh/config` を読まない。

**Tech Stack:** Go 1.26, `golang.org/x/crypto/ssh`（既に直接の依存）, Echo v5, React 19, Vitest, Playwright

## Global Constraints

- 仕様は `docs/superpowers/specs/2026-08-13-in-process-ssh-client-design.md`
- **接続のために外部プログラムを起こさない。** `ProxyCommand` は断る
- **秘密をログにも応答にも端末のエコーにも出さない**
- 新しい文言は `web/src/i18n/messages` の en と ja の両方へ
- Linux は Docker で確かめる（`GOOS=linux go vet` では足りない）
- コミット前に `git diff --cached` そのものを読む（別セッションが同じ木を編集する）
- テストは実リモートへ接続しない。プロセス内の SSH サーバーを `net.Pipe` の上に立てる

---

### Task 1: 継ぎ目 — `terminal.Spec.Open`

**Files:**
- Modify: `internal/terminal/registry.go`
- Test: `internal/terminal/registry_test.go`

**Interfaces:**
- Produces: `Spec.Open func(Size) (Process, error)`

`Open` が設定されていれば `Registry.Open` はそれを呼び、`Starter` を使わない。両方 nil なら `ErrNoStarter`。

- [ ] **Step 1:** `Spec` に `Open` を足し、`Registry.Open` の `r.Start.Start(...)` を分岐させる
- [ ] **Step 2:** テスト `TestRegistryUsesTheSpecOwnOpenerWhenItHasOne` — `Start` を nil にしたレジストリでも `Open` があれば開けること、`Cleanup` が失敗時に呼ばれること
- [ ] **Step 3:** `go test ./internal/terminal/`
- [ ] **Step 4:** commit

---

### Task 2: `Target` と、解決済みの値からの組み立て

**Files:**
- Create: `internal/sshclient/target.go`, `internal/sshclient/target_test.go`

**Interfaces:**
- Produces: `Target`, `Methods`, `NewTarget(values effective.Values, alias string, home string) (Target, []Notice, error)`, `Notice`, `ErrProxyCommand`

`effective.Values` から `Target` を作る。`ProxyJump` は連鎖に展開する（`user@host:port` を分解し、それぞれについて再び解決する必要があるので、呼び出し側から解決関数を受け取る）。`ProxyCommand` があれば `ErrProxyCommand`。notice の表は spec の決定事項 6。

- [ ] **Step 1:** テストを書く — 値だけの `Target`、`ProxyJump` の連鎖、`ProxyCommand` の拒否、notice の一覧、`IdentityFile` の `~` 展開
- [ ] **Step 2:** 実装
- [ ] **Step 3:** `go test ./internal/sshclient/`
- [ ] **Step 4:** commit

---

### Task 3: テスト用のプロセス内 SSH サーバー

**Files:**
- Create: `internal/sshclient/testserver_test.go`

**Interfaces:**
- Produces:（テスト内のみ）`newTestServer(t, options) *testServer`, `testServer.Dial() net.Conn`, `.HostKey`, `.Authorised`, `.LastPTY`, `.LastEnv`, `.ExitCode`

`net.Pipe` の上で `ssh.NewServerConn` を回す。公開鍵・パスワード・keyboard-interactive を受け付け、session チャンネルで `pty-req`、`window-change`、`env`、`shell`、`exec` を記録する。

- [ ] **Step 1:** サーバーを書き、`TestTheTestServerCompletesAHandshake` で自分自身を検査する
- [ ] **Step 2:** `go test ./internal/sshclient/`
- [ ] **Step 3:** commit

---

### Task 4: ホスト鍵の検証

**Files:**
- Create: `internal/sshclient/hostkey.go`, `internal/sshclient/hostkey_test.go`

**Interfaces:**
- Produces: `HostKeys{Path string, Ask func(Question) (bool, error), Add func(host string, key ssh.PublicKey) error}`, `(HostKeys).Callback(target Target) ssh.HostKeyCallback`, `ErrHostKeyChanged`, `ErrHostKeyUnknown`

spec の表のとおり。一致→通す、不一致→`ErrHostKeyChanged`（尋ねない）、未知→`StrictHostKeyChecking` で分岐。読むのは `internal/knownhosts.ParseFile` と `MatchesHost`。

- [ ] **Step 1:** テスト — 一致、不一致で断ること、未知で尋ねること、`accept-new` で書くこと、`yes` で断ること
- [ ] **Step 2:** 実装
- [ ] **Step 3:** `go test ./internal/sshclient/`
- [ ] **Step 4:** commit

---

### Task 5: 認証

**Files:**
- Create: `internal/sshclient/auth.go`, `internal/sshclient/auth_test.go`

**Interfaces:**
- Produces: `Prompter interface { Line(prompt string) (string, error); Secret(prompt string) (string, error); Confirm(prompt string) (bool, error) }`, `Auth{Identities []string, Agent net.Conn, Passphrase func(path string) (string, bool), Prompt Prompter}`, `(Auth).Methods(target Target) []ssh.AuthMethod`

鍵はファイルから読み、`keys.DecodePrivateKey` で復号する。パスフレーズは vault → 端末の順。agent は `SSH_AUTH_SOCK`。順序は `PreferredAuthentications`、無ければ publickey → keyboard-interactive → password。

- [ ] **Step 1:** テスト — パスフレーズ無しの鍵、vault のパスフレーズ、尋ねたパスフレーズ、agent、password、keyboard-interactive、`IdentitiesOnly` が agent を飛ばすこと、**答えが端末へエコーされないこと**
- [ ] **Step 2:** 実装
- [ ] **Step 3:** `go test ./internal/sshclient/`
- [ ] **Step 4:** commit

---

### Task 6: 端末への橋渡し

**Files:**
- Create: `internal/sshclient/prompt.go`, `internal/sshclient/prompt_test.go`

**Interfaces:**
- Produces: `StreamPrompter{Out io.Writer, In io.Reader}` が `Prompter` を満たす

出力へプロンプトを書き、入力から 1 行読む。`Secret` はエコーしない。`Confirm` は `yes`/`no` を受ける（OpenSSH と同じ文言に寄せる）。CR と LF の両方を行末として扱う——ブラウザの端末は CR を送る。

- [ ] **Step 1:** テスト — 行の読み取り、CR 終端、エコーしないこと、`yes`/`no`、途中で閉じられた入力
- [ ] **Step 2:** 実装
- [ ] **Step 3:** `go test ./internal/sshclient/`
- [ ] **Step 4:** commit

---

### Task 7: セッション — `terminal.Process` の実装

**Files:**
- Create: `internal/sshclient/session.go`, `internal/sshclient/session_test.go`

**Interfaces:**
- Produces: `Session` が `terminal.Process` を満たす

`Read` は stdout と stderr を合流させる。`Resize` は `window-change`。`Hangup` はチャンネルを閉じる。`Wait` はリモートの終了コード。`SetEnv` は `env` リクエスト。`RemoteCommand` があれば `exec`、無ければ `shell`。`ServerAliveInterval` があれば `keepalive@openssh.com` を送る。

- [ ] **Step 1:** テスト — 読み書き、stderr の合流、`window-change` が届くこと、終了コード、`env`、`exec` と `shell` の分岐、keepalive
- [ ] **Step 2:** 実装
- [ ] **Step 3:** `go test ./internal/sshclient/`
- [ ] **Step 4:** commit

---

### Task 8: Dial — 全部を繋ぐ

**Files:**
- Create: `internal/sshclient/client.go`, `internal/sshclient/client_test.go`
- Create: `internal/sshclient/jump.go`

**Interfaces:**
- Produces: `Dialer{Auth Auth, HostKeys HostKeys, Dial func(ctx, network, address string) (net.Conn, error)}`, `(Dialer).Open(ctx context.Context, target Target, size terminal.Size, prompt Prompter) (terminal.Process, error)`

`ProxyJump` の連鎖は手前から順に繋ぎ、各段の `ssh.Client.Dial` を次の段の `net.Conn` にする。

- [ ] **Step 1:** テスト — 直接接続、2 段の `ProxyJump`、認証失敗の理由、ホスト鍵不一致で接続しないこと、`ConnectTimeout`
- [ ] **Step 2:** 実装
- [ ] **Step 3:** `go test ./internal/sshclient/`
- [ ] **Step 4:** commit

---

### Task 9: 配線 — httpserver の SSH の枝

**Files:**
- Modify: `internal/httpserver/terminal.go`
- Modify: `internal/app/*`（合成の根）
- Test: `internal/httpserver/terminal_test.go`, `internal/acceptance/*`

`spec()` の SSH の枝が `platform.InteractiveSSH` の代わりに `Dialer` を組み立て、`Spec.Open` へ渡す。`effective.Resolve` の `Refusal` は、セッションを作らずに理由を返す。

- [ ] **Step 1:** テスト — 解決が拒んだ alias でセッションが作られないこと、`ProxyCommand` を持つ設定が断られること、**この経路が外部プロセスを一度も起こさないこと**
- [ ] **Step 2:** 実装
- [ ] **Step 3:** `go test ./...`
- [ ] **Step 4:** commit

---

### Task 10: e2e、README、ゲート

**Files:**
- Modify: `web/e2e/terminal.spec.ts`
- Modify: `README.md`

- [ ] **Step 1:** e2e — プロセス内 SSH サーバーへ繋いでコマンドを走らせ、出力を見る
- [ ] **Step 2:** README の「SSH 実行の境界」を書き換える（外部 `ssh` は 3 経路だけになったこと、対話セッションが PTY を確保しなくなったこと、notice の表）
- [ ] **Step 3:** `go test ./...`、`make verify-generated`、`make e2e`、Docker Linux、`make fuzz`
- [ ] **Step 4:** commit
