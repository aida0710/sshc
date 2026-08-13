# 外部の ssh を退ける 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 接続のために OpenSSH のプログラムを起こすのをやめ、`SSH_ASKPASS` の一式を撤去する。

**Architecture:** 残る 4 経路（認証テスト、公開鍵のリモート登録、ホスト鍵の取得、CLI）を B2 の `internal/sshclient` へ移す。**新しい SSH の話し手は作らない。** 移し終えると askpass が要らなくなるので、最後にまとめて消す。

**Tech Stack:** Go 1.26, `golang.org/x/crypto/ssh`, `golang.org/x/term`（どちらも既に直接の依存）

## Global Constraints

- 仕様は `docs/superpowers/specs/2026-08-13-retiring-the-external-ssh-design.md`
- **接続のために外部プログラムを起こさない**
- 秘密をログにも応答にも出さない。応答の文字列はホームを `~` に置き換えてから
- 新しい文言は `web/src/i18n/messages` の en と ja の両方へ
- Linux は Docker で確かめる
- コミット前に `git diff --cached` そのものを読む
- テストは B2 のプロセス内 SSH サーバーを使う。実リモートへは接続しない
- **消すのは最後（Task 6）。** それまでは並走させ、何も壊さない

---

### Task 1: ホスト鍵の取得をプロセス内へ

**Files:**
- Create: `internal/sshclient/scan.go`, `internal/sshclient/scan_test.go`
- Modify: `internal/knownhosts/scan.go`, `internal/knownhosts/scan_test.go`

**Interfaces:**
- Produces: `sshclient.ScanHostKeys(ctx, dial DialFunc, address string, timeout time.Duration) ([]ssh.PublicKey, error)`, `sshclient.ScanAlgorithms` (既定の種別の並び)

鍵種別ごとに `HostKeyAlgorithms` を 1 つに絞って握手を始め、`HostKeyCallback` が鍵を受け取ったところで断る。認証には進まない。`knownhosts.Scanner` は `Runner`/`Toolchain` を手放す。

- [ ] **Step 1:** テスト — 鍵が取れること、サーバーが持たない種別を飛ばすこと、**認証しないこと**（サーバーは鍵を一つも受け付けない設定にする）、届かない宛先
- [ ] **Step 2:** 実装
- [ ] **Step 3:** `go test ./internal/sshclient/ ./internal/knownhosts/`
- [ ] **Step 4:** commit

---

### Task 2: 認証テストをプロセス内へ

**Files:**
- Create: `internal/sshclient/probe.go`, `internal/sshclient/probe_test.go`
- Modify: `internal/diagnostics/authentication.go`, `internal/diagnostics/service.go`

**Interfaces:**
- Produces: `sshclient.Probe{Method, Banner, Tried}`, `(Dialer).Probe(ctx, target) (Probe, error)`

接続して認証だけを試し、チャンネルを開かずに閉じる。**`Prompter` を渡さない**ので、端末の問いは出ない。ホスト鍵は `StrictHostKeyChecking=yes` 相当。`HardeningOptions` は削除。

- [ ] **Step 1:** テスト — 通る（方式が名指しされる）、通らない、未知のホストで断る、**尋ねないこと**
- [ ] **Step 2:** 実装と、`diagnostics.Authentication` の差し替え
- [ ] **Step 3:** `go test ./internal/sshclient/ ./internal/diagnostics/ ./internal/httpserver/`
- [ ] **Step 4:** commit

---

### Task 3: 公開鍵のリモート登録をプロセス内へ

**Files:**
- Create: `internal/sshclient/exec.go`, `internal/sshclient/exec_test.go`
- Modify: `internal/remotekey/register.go`

**Interfaces:**
- Produces: `sshclient.Output{Stdout, Stderr, ExitCode, Truncated}`, `(Dialer).Run(ctx, target, command string, stdin []byte) (Output, error)`

`exec` チャンネルで走らせる。鍵は stdin を通り、**argv には決して乗らない。** 出力は上限つき。`writeConfigSnapshot` は削除。

- [ ] **Step 1:** テスト — stdin が届くこと、**argv に鍵が乗らないこと**、終了コード、出力の上限
- [ ] **Step 2:** 実装と `remotekey.Service` の差し替え
- [ ] **Step 3:** `go test ./internal/sshclient/ ./internal/remotekey/ ./internal/httpserver/`
- [ ] **Step 4:** commit

---

### Task 4: ローカル端末を SSH へ繋ぐ

**Files:**
- Create: `internal/sshclient/tty.go`, `internal/sshclient/tty_test.go`

**Interfaces:**
- Produces: `sshclient.Attach(ctx, process terminal.Process, in *os.File, out io.Writer) (int, error)`

`x/term` で raw にし、`SIGWINCH` で大きさを送り直す。**終了時に必ず戻す**——`defer` に置き、panic でも通ること。テレタイプでない入力では raw にせず、大きさも問い合わせない。

- [ ] **Step 1:** テスト — テレタイプでない入力で raw にしないこと、終了コードが伝わること、読み書きが通ること
- [ ] **Step 2:** 実装
- [ ] **Step 3:** `go test ./internal/sshclient/`
- [ ] **Step 4:** commit

---

### Task 5: CLI をプロセス内へ

**Files:**
- Modify: `cmd/sshc/connect.go`, `cmd/sshc/connect_test.go`
- Modify: `internal/httpserver/connect.go`（返すものを変える）
- Modify: `api/openapi.yaml`（`/cli/connect` の応答）

`/cli/connect` は、その接続に使う鍵のパスフレーズを返す。届かなければ端末で尋ねる。`platform.InteractiveSSH` の呼び出しを外す（削除は Task 6）。

- [ ] **Step 1:** テスト — sshc が走っていないときも接続を試みること、パスフレーズが argv にも環境にも乗らないこと、`ProxyCommand` を断ること
- [ ] **Step 2:** 実装
- [ ] **Step 3:** `go test ./...`、`make verify-generated`
- [ ] **Step 4:** commit

---

### Task 6: askpass を消す

**Files:**
- Delete: `cmd/sshc/askpass.go`, `cmd/sshc/askpass_test.go`
- Modify: `internal/platform/interactive.go`（`InteractiveSSH`、`FreezeSSHConfig`、5 つの変数を削除）
- Modify: `internal/httpserver/password.go`（`Askpass` エンドポイントと prompt 照合）
- Modify: `internal/secret/service.go`（トークン一式）
- Modify: `internal/platform/command.go`（`Toolchain.KeyScan`）
- Modify: `internal/app/run.go`, `internal/httpserver/server.go`（Options の整理）
- Modify: `api/openapi.yaml`

**消せることの証明が先である。** 消す前に、`internal/acceptance` に「接続のどの経路も外部プロセスを起こさない」を置く。

- [ ] **Step 1:** `internal/acceptance` に全経路の検査を置き、緑にする
- [ ] **Step 2:** 上の一覧を削除する
- [ ] **Step 3:** `go test ./...`、`make verify-generated`
- [ ] **Step 4:** commit

---

### Task 7: README とゲート

**Files:**
- Modify: `README.md`

- [ ] **Step 1:** 「SSH 実行の境界」を書き換える（接続のために起こす外部プログラムが無くなったこと、残る 3 つが接続ではないこと、`SSH_ASKPASS` の節の削除、`make install` の理由の変更）
- [ ] **Step 2:** `go test ./...`、`make verify-generated`、`make e2e`、Docker Linux、`make fuzz`
- [ ] **Step 3:** commit
