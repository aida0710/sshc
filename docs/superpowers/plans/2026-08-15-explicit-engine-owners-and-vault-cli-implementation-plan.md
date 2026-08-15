# Explicit Engine Owners and Vault CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** macOS と Linux で、bare `sshc`・desktop・headless の責務を明示し、単一エンジン、Vault CLI、解錠待ち接続、安定した CLI 配置を実装する。

**Architecture:** コマンド解析を副作用から分離し、`sshc engine` と `sshc headless` だけが共通の engine runner に入る。Electron は stdin の ownership pipe で desktop engine を所有し、headless は端末または外部 supervisor が foreground process を所有する。CLI は versioned handoff を発見にだけ使い、認証済み `/cli/status` を生存・owner・protocol の正本として接続と Vault 操作を振り分ける。

**Tech Stack:** Go 1.26 / Echo v5 / `golang.org/x/term` / Electron 43 / Node `node:test`

**Spec:** `docs/superpowers/specs/2026-08-15-explicit-desktop-headless-windows-design.md`

## Global Constraints

- この計画を Windows platform plan、native CI/release plan より先に実行する。
- `--own-engine`、bare-engine 起動、旧 handoff、旧 CLI protocol の互換 shim は作らない。
- 現行の SSH config、vault version 2/3、metadata、backup、sync snapshot の reader は維持する。
- bare `sshc` と `sshc <alias>` は engine lock を取得しない。
- Vault password は TTY から no-echo で読み、argv、環境変数、通常 stdin pipe、log に載せない。
- `vault lock` は既存 session を終了しない。engine 終了は全 session を終了する。
- コメントは日本語で「なぜ」を説明する。
- package をローカルへ追加インストールしない。既存の `x/term` と `x/sys` を使う。
- 各タスクは対象テスト、`go test ./...` または `npm test --prefix desktop`、`go build ./...` を通してから commit する。
- 実行中の開発用 Electron/engine process はテスト用 HOME と専用 state directory を使わずに停止・再利用しない。

---

### Task 1: コマンド解析を明示的な invocation にする

**Files:**
- Create: `cmd/sshc/invocation.go`
- Create: `cmd/sshc/invocation_test.go`
- Modify: `cmd/sshc/main.go`
- Modify: `cmd/sshc/connect.go`
- Modify: `cmd/sshc/status.go`

**Interfaces:**

```go
type invocationKind uint8

const (
	invocationInvalid invocationKind = iota
	invocationDesktop
	invocationEngine
	invocationHeadless
	invocationConnect
	invocationChoose
	invocationList
	invocationOpen
	invocationStatus
	invocationVault
	invocationHelp
)

type invocation struct {
	Kind invocationKind
	Args []string
}

func parseInvocation(argv []string) (invocation, error)
```

- [ ] **Step 1: 予約語、bare invocation、余分な引数を表にした失敗テストを書く**

`cmd/sshc/invocation_test.go` で少なくとも次を検査する。

```go
func TestParseInvocationSeparatesOwnersFromDesktopActivation(t *testing.T) {
	tests := []struct {
		argv []string
		kind invocationKind
	}{
		{[]string{"sshc"}, invocationDesktop},
		{[]string{"sshc", "engine"}, invocationEngine},
		{[]string{"sshc", "headless"}, invocationHeadless},
		{[]string{"sshc", "server-a"}, invocationConnect},
		{[]string{"sshc", "vault", "unlock"}, invocationVault},
	}
	for _, test := range tests {
		got, err := parseInvocation(test.argv)
		if err != nil || got.Kind != test.kind {
			t.Fatalf("parseInvocation(%q) = %#v, %v", test.argv, got, err)
		}
	}
}
```

`--own-engine`、`engine extra`、未知の `vault` action、3語以上の裸 alias は usage error になることも検査する。

- [ ] **Step 2: テストが未定義で落ちることを確認する**

Run: `go test ./cmd/sshc -run 'TestParseInvocation'`
Expected: FAIL（`parseInvocation` が未定義）

- [ ] **Step 3: parser と新しい usage を実装する**

`flag` package と `ownEngine` global を削除し、全予約語を `parseInvocation` の一箇所で判定する。usage の owner 部分は次の意味を明記する。

```text
sshc                 launch or focus the desktop application
sshc engine          internal: run the engine owned by Electron
sshc headless        run a foreground engine for terminals and supervisors
```

`engine` の説明には「Electron が子 engine の lifetime を所有するために存在し、利用者が通常直接実行しない」という理由を書く。

- [ ] **Step 4: `main` を parse-then-dispatch に縮める**

`main()` は home と共通 client を用意し、kind ごとの関数を呼んで exit code を返す。engine を既定分岐に置かない。`connectInvocation`、`helpInvocation`、`openInvocation`、`tuiInvocation` は parser へ統合し、その個別テストを table test へ移す。

- [ ] **Step 5: parser と既存 CLI のテストを通す**

Run: `go test ./cmd/sshc`
Expected: PASS

- [ ] **Step 6: build と commit**

Run: `go build ./...`
Expected: PASS

```bash
git add cmd/sshc/invocation.go cmd/sshc/invocation_test.go cmd/sshc/main.go cmd/sshc/connect.go cmd/sshc/status.go
git commit -m "refactor: make engine ownership commands explicit"
```

---

### Task 2: handoff を versioned atomic document にする

**Files:**
- Modify: `internal/handoff/handoff.go`
- Modify: `internal/handoff/handoff_test.go`
- Modify: `internal/app/run.go`
- Modify: `internal/app/run_test.go`
- Modify: `cmd/sshc/status.go`
- Modify: `cmd/sshc/status_test.go`

**Interfaces:**

```go
type Owner string

const (
	OwnerDesktop  Owner = "desktop"
	OwnerHeadless Owner = "headless"
	SchemaVersion = 1
	ProtocolVersion = 1
)

type Handoff struct {
	SchemaVersion   int    `json:"schemaVersion"`
	URL             string `json:"url"`
	Secret          string `json:"secret"`
	Owner           Owner  `json:"owner"`
	PID             int    `json:"pid"`
	Version         string `json:"version"`
	ProtocolVersion int    `json:"protocolVersion"`
}

func Write(directory string, document Handoff) error
func Read(directory string) (Handoff, error)
func Remove(directory, secret string) error
```

- [ ] **Step 1: schema、exact protocol、atomic replacement、secret-safe cleanup の失敗テストを書く**

検査項目:

- field が欠けた旧 `{url,secret}` を拒否する。
- unknown owner、schema/protocol mismatch、非 loopback URL、空 secret、非正 PID を拒否する。
- 書込み後は JSON が一度で decode でき、Unix で directory 0700 / file 0600 になる。
- 旧 document と異なる secret の `Remove` は消さず、同じ secret だけ消す。
- temp file が残らない。

- [ ] **Step 2: 現在の実装が schema テストで落ちることを確認する**

Run: `go test ./internal/handoff`
Expected: FAIL

- [ ] **Step 3: Unix atomic writer を実装する**

同じ directory 内に 0600 の一時ファイルを作り、JSON encode、`Sync`、`Close`、`Rename`、directory `Sync` の順に進める。途中失敗では temp を削除する。`Read` は parse 後に全 field を validate する。

- [ ] **Step 4: app dependency に owner と process metadata を渡す**

```go
type Dependencies struct {
	// existing fields
	Owner handoff.Owner
	PID   int
}
```

`app.Build` が listener 準備後に current version と protocol を含む document を書き、`app.Run` の cleanup は URL でなく secret で ownership を照合する。`PID == 0` の test dependency には injected PID provider を使い、production だけ `os.Getpid()` を渡す。

- [ ] **Step 5: CLI read path で protocol mismatch を行動可能な error にする**

`engineStatus`、`runOpen`、接続 client が同じ `readHandoff` helper を使い、mismatch は「running app と CLI を同じ版にして app を再起動する」と出す。旧 format へ fallback しない。

- [ ] **Step 6: テスト、build、commit**

Run: `go test ./internal/handoff ./internal/app ./cmd/sshc`
Expected: PASS

Run: `go build ./...`
Expected: PASS

```bash
git add internal/handoff internal/app/run.go internal/app/run_test.go cmd/sshc/status.go cmd/sshc/status_test.go
git commit -m "feat: version the command handoff atomically"
```

---

### Task 3: owner-aware status と Vault CLI HTTP routes を実装する

**Files:**
- Modify: `internal/httpserver/connect.go`
- Modify: `internal/httpserver/connect_test.go`
- Create: `internal/httpserver/vault_cli.go`
- Create: `internal/httpserver/vault_cli_test.go`
- Modify: `internal/httpserver/server.go`
- Modify: `internal/httpserver/password.go`
- Modify: `internal/httpserver/password_test.go`
- Modify: `internal/app/run.go`

**Interfaces:**

```go
const (
	VaultStatusPath   = "/cli/vault/status"
	VaultCreatePath   = "/cli/vault/create"
	VaultUnlockPath   = "/cli/vault/unlock"
	VaultLockPath     = "/cli/vault/lock"
	VaultChangePath   = "/cli/vault/change-password"
)

type CLIStatus struct {
	Owner           handoff.Owner `json:"owner"`
	Version         string        `json:"version"`
	ProtocolVersion int           `json:"protocolVersion"`
	Vault           bool          `json:"vault"`
	Unlocked        bool          `json:"unlocked"`
	Sessions        int           `json:"sessions"`
}
```

- [ ] **Step 1: route contract の失敗テストを書く**

全 route について handoff header が空・不一致なら 401、oversized JSON は 413、未知 field は 400 を検査する。さらに以下を検査する。

- status は owner/version/protocol と `missing|locked|unlocked` を構成できる data を返す。
- create は既存 vault を 409、短い password を 400、成功を 204 にする。
- unlock は missing を 409、誤 password を詳細を漏らさず 401、成功を 204 にする。
- lock は session count を変更せず 204 にする。
- change は事前 lock を 409、current password 不一致を 401、local success + remote reseal failure を 207 と構造化 body で返す。

- [ ] **Step 2: route 未登録で落ちることを確認する**

Run: `go test ./internal/httpserver -run 'TestCLIVault|TestCLIStatusIncludesOwner'`
Expected: FAIL

- [ ] **Step 3: CLI 認証と strict JSON decoder を共通化する**

`ConnectHandlers` の constant-time secret comparison を private helper に切り出し、既存 `/cli/connect`、`/cli/open`、`/cli/status` と新 route で共用する。browser middleware の cookie/CSRF contract は変更しない。

- [ ] **Step 4: Vault service operation を browser/CLI 間で共用する**

`PasswordHandlers.Change` 内の local rekey + `ResealSnapshot` を、HTTP に依存しない `changeMasterPassword(ctx,current,next)` result へ抽出する。CLI と browser は同じ operation を呼び、remote partial failure の semantics を一致させる。

- [ ] **Step 5: startup wiring を追加する**

`internal/app/run.go` から `*secret.Service`、owner、version、protocol、session count、snapshot resealer を `ConnectHandlers` と Vault handlers に渡す。status read は vault inactivity timer を更新しない。

- [ ] **Step 6: lock が live sessions を閉じない回帰テストを通す**

test session count を 1 にして `/cli/vault/lock` を呼び、vault が locked でも session count が 1 のままで terminal registry の close callback が未呼出しであることを確認する。

- [ ] **Step 7: テスト、build、commit**

Run: `go test ./internal/httpserver ./internal/app ./internal/secret`
Expected: PASS

Run: `go build ./...`
Expected: PASS

```bash
git add internal/httpserver internal/app/run.go internal/secret
git commit -m "feat: expose authenticated vault operations to the cli"
```

---

### Task 4: TTY-only Vault CLI client を実装する

**Files:**
- Create: `cmd/sshc/vault.go`
- Create: `cmd/sshc/vault_test.go`
- Modify: `cmd/sshc/invocation.go`
- Modify: `cmd/sshc/main.go`
- Modify: `cmd/sshc/status.go`

**Interfaces:**

```go
type passwordTerminal interface {
	IsTerminal(fd int) bool
	ReadPassword(fd int) ([]byte, error)
}

func runVault(
	ctx context.Context,
	action string,
	stateDir string,
	client *http.Client,
	stdin *os.File,
	stdout, stderr io.Writer,
	terminal passwordTerminal,
) int
```

- [ ] **Step 1: command table と secret leak の失敗テストを書く**

`httptest.Server` と fake terminal を使い、次を検査する。

- `status` は TTY 不要で `engine: desktop` と `vault: locked` を出す。
- create は prompt 前に status を読み、existing vault なら一文字も読まない。
- create と change は confirmation mismatch 時に request を送らない。
- unlock が already unlocked なら prompt しない。
- password-required action は non-TTY で 1、server request は 0 件。
- password canary が stdout/stderr/request error に出ない。
- Ctrl-C 相当の context cancellation は 130。

- [ ] **Step 2: client 未実装で落ちることを確認する**

Run: `go test ./cmd/sshc -run 'TestRunVault'`
Expected: FAIL

- [ ] **Step 3: x/term adapter と zeroing を実装する**

production adapter は `term.IsTerminal` と `term.ReadPassword` を呼ぶ。読み取った `[]byte` は JSON encode 後に全 byte を 0 にし、string を長寿命 state に保持しない。prompt は stderr、human-readable result は stdout に出す。

- [ ] **Step 4: action ごとの preflight を実装する**

- create: missing のときだけ new + confirmation。
- unlock: locked のときだけ一回。
- lock: prompt なし。
- change-password: unlocked のときだけ current + new + confirmation。

HTTP status を public exit contract の 0/1/2/130 に写像し、partial remote reseal は local success を明記して code 1 にする。

- [ ] **Step 5: 全 CLI テスト、build、commit**

Run: `go test ./cmd/sshc`
Expected: PASS

Run: `go build ./...`
Expected: PASS

```bash
git add cmd/sshc/vault.go cmd/sshc/vault_test.go cmd/sshc/invocation.go cmd/sshc/main.go cmd/sshc/status.go
git commit -m "feat: add tty-only vault commands"
```

---

### Task 5: 共通 engine runner と明示的 ownership channel を実装する

**Files:**
- Create: `cmd/sshc/engine.go`
- Create: `cmd/sshc/engine_test.go`
- Create: `cmd/sshc/signals_unix.go`
- Modify: `cmd/sshc/main.go`
- Modify: `cmd/sshc/lock.go`
- Modify: `cmd/sshc/lock_test.go`
- Delete: `cmd/sshc/watch.go`
- Delete: `cmd/sshc/watch_test.go`
- Modify: `internal/app/run.go`

**Interfaces:**

```go
type engineMode uint8

const (
	engineDesktop engineMode = iota + 1
	engineHeadless
)

func runEngine(
	ctx context.Context,
	mode engineMode,
	home string,
	ownership io.Reader,
	stdout, stderr io.Writer,
) int
```

- [ ] **Step 1: ownership と mode の失敗テストを書く**

- desktop は ownership が nil、TTY、開始前 EOF の場合に lock を取らず code 1。
- desktop は ownership EOF で context を cancel し graceful shutdown へ進む。
- headless は ownership を読まず foreground で signal context まで生きる。
- lock conflict は desktop だけ internal code 3、headless は public code 1。
- headless announcement は locked/missing の操作案内だけで bootstrap URL/token を含まない。

- [ ] **Step 2: current parent watcher 前提でテストが落ちることを確認する**

Run: `go test ./cmd/sshc -run 'TestRunEngine|TestDesktopOwnership'`
Expected: FAIL

- [ ] **Step 3: runner と ownership cancellation を実装する**

desktop では lock より前に pipe を validate し、engine 起動後は blocking read の EOF または read error を cancellation とする。PID を poll しない。headless は stdin を password channel としても ownership としても使用しない。

- [ ] **Step 4: signal construction を Unix file に分ける**

`signals_unix.go` に `signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)` を置き、`main.go` から Unix-only `syscall.SIGTERM` を除く。Windows implementation は次計画で同じ interface を満たす。

- [ ] **Step 5: startup/shutdown order を固定する**

engine lock → app services/listener → handoff → serve/announce の順で開始し、stopping flag → owner-safe handoff remove → terminals/WebSockets/forwards close → 5秒 HTTP shutdown → vault lock → engine lock release の順で終了する。`app.Run` の error path でも同じ cleanup を通す。

- [ ] **Step 6: parent watcher を削除し回帰テストを更新する**

`watchParent` と PID 監視の test を削除する。inspection-based orphan 判定が残っていないことを確認する。

Run: `rg -n 'watchParent|Getppid|parentTick' cmd internal desktop`
Expected: no matches

- [ ] **Step 7: テスト、race、build、commit**

Run: `go test ./cmd/sshc ./internal/app`
Expected: PASS

Run: `go test -race ./cmd/sshc ./internal/app ./internal/httpserver`
Expected: PASS

Run: `go build ./...`
Expected: PASS

```bash
git add cmd/sshc internal/app/run.go
git commit -m "feat: separate desktop and headless engine ownership"
```

---

### Task 6: macOS/Linux desktop activation adapter を実装する

**Files:**
- Create: `cmd/sshc/desktop.go`
- Create: `cmd/sshc/desktop_test.go`
- Modify: `cmd/sshc/launch_darwin.go`
- Replace: `cmd/sshc/launch_other.go`
- Create: `cmd/sshc/launch_linux.go`
- Create: `cmd/sshc/launch_unsupported.go`

**Interfaces:**

```go
type desktopLauncher interface {
	Available() (bool, error)
	Launch(context.Context) error
}

func runDesktop(
	ctx context.Context,
	stateDir string,
	client *http.Client,
	launcher desktopLauncher,
	stderr io.Writer,
) int
```

- [ ] **Step 1: owner routing の失敗テストを書く**

- live desktop: launcher を一回呼び focus させる。
- live headless: launcher を呼ばず stop headless の案内で code 1。
- stale handoff + graphical launcher: launch する。
- no engine + no display: `sshc headless` を案内する。
- launcher error: absolute target の修復方法だけを出し PATH search しない。

- [ ] **Step 2: test が launch bool interface では表現できず落ちることを確認する**

Run: `go test ./cmd/sshc -run 'TestRunDesktop'`
Expected: FAIL

- [ ] **Step 3: macOS adapter を修正する**

`/usr/bin/open -b com.github.aida0710.sshc` を `exec.CommandContext` で直接実行する。user-driven activation では `--hidden` を渡さない。

- [ ] **Step 4: Linux descriptor adapter を実装する**

`~/.ssh/sshc/desktop.json` を読み、DISPLAY または WAYLAND_DISPLAY があり、記録 path が absolute executable regular file であるときだけ直接 exec する。shell と PATH search は使わない。AppImage が移動済みなら「新しい場所で AppImage を一度開く」と出す。

- [ ] **Step 5: bare `sshc` を `runDesktop` に接続する**

bare invocation は engine runner、`runOpen`、handoff print のいずれにも入らない。live desktop でも launcher を介して Electron の second-instance focus を使う。

- [ ] **Step 6: テスト、build、commit**

Run: `go test ./cmd/sshc`
Expected: PASS

Run: `go build ./...`
Expected: PASS

```bash
git add cmd/sshc/desktop.go cmd/sshc/desktop_test.go cmd/sshc/launch_darwin.go cmd/sshc/launch_linux.go cmd/sshc/launch_unsupported.go cmd/sshc/launch_other.go
git commit -m "feat: activate desktop explicitly on macos and linux"
```

---

### Task 7: owner-aware connection state machine を実装する

**Files:**
- Modify: `cmd/sshc/connect.go`
- Modify: `cmd/sshc/connect_test.go`
- Modify: `cmd/sshc/status.go`
- Modify: `internal/httpserver/connect.go`
- Modify: `internal/httpserver/connect_test.go`

**Interfaces:**

```go
type engineProbe interface {
	Status(context.Context) (statusAnswer, error)
	Connection(context.Context, string) (connectAnswer, error)
}

func waitForDesktopUnlock(
	ctx context.Context,
	initial handoff.Handoff,
	probe engineProbe,
	poll time.Duration,
) error
```

- [ ] **Step 1: 六つの connection flow と cancellation の失敗テストを書く**

設計 section 8.2 の desktop unlocked、desktop locked、no engine graphical、headless unlocked、headless locked、no engine headless を table-driven fake で検査する。追加で以下を検査する。

- desktop locked は launcher を呼んでから無期限に poll する。
- desktop vault missing は launcher を呼び、`vault create` を案内して同じ engine が create + unlock されるまで待つ。
- headless vault missing は待たず `sshc vault create` を案内する。
- unlock 後は元 alias を一回だけ connection request し SSH を開始する。
- Ctrl-C は 130。
- owner/secret/protocol change と engine exit は code 1。
- headless locked は待たず `sshc vault unlock` を案内する。
- engine 不在時に `app.NewCLIConnection` を呼ばず、secretless fallback をしない。

- [ ] **Step 2: current fallback behavior で落ちることを確認する**

Run: `go test ./cmd/sshc -run 'TestRunConnect|TestWaitForDesktopUnlock'`
Expected: FAIL

- [ ] **Step 3: status-first routing を実装する**

handoff read 後すぐ `/cli/status` を認証付きで読み、live response の owner を正本にする。no engine なら launcher availability を見て desktop を起動し、handoff が現れるまで context-aware に待つ。

- [ ] **Step 4: desktop unlock wait を実装する**

固定 timeout を置かず、250ms 程度の ticker と context で status を再取得する。poll ごとに original secret、owner desktop、protocol exact match を確認する。focus は一回だけで、各 poll では起動しない。

- [ ] **Step 5: inline master-password prompt と fallback を削除する**

`locked` と `unlock` helper および `runConnect` 内の `term.ReadPassword` を削除する。password input は `sshc vault ...` に集約する。engine がない/locked のままでは SSH config を直接開いて接続しない。

- [ ] **Step 6: real SSH client injection を保ち既存 auth tests を通す**

engine が unlocked と確認できた後だけ現在の saved key/account password mapping を構成する。2FA など保存対象外の prompt は既存 `sshclient.Attach` 経路で interactive のままにする。

- [ ] **Step 7: テスト、race、build、commit**

Run: `go test ./cmd/sshc ./internal/httpserver ./internal/sshclient`
Expected: PASS

Run: `go test -race ./cmd/sshc ./internal/httpserver`
Expected: PASS

Run: `go build ./...`
Expected: PASS

```bash
git add cmd/sshc/connect.go cmd/sshc/connect_test.go cmd/sshc/status.go internal/httpserver/connect.go internal/httpserver/connect_test.go
git commit -m "feat: route connections through the live engine owner"
```

---

### Task 8: Electron に ownership pipe と background lifetime を持たせる

**Files:**
- Modify: `desktop/main.js`
- Modify: `desktop/entrance.js`
- Modify: `desktop/entrance.test.js`
- Modify: `desktop/reopen.js`
- Modify: `desktop/reopen.test.js`
- Create: `desktop/lifecycle.js`
- Create: `desktop/lifecycle.test.js`
- Modify: `desktop/package.json`

**Interfaces:**

```js
function engineSpawnOptions() {
  return { stdio: ["pipe", "pipe", "pipe"], windowsHide: true };
}

async function stopOwnedEngine(child, timeoutMs = 5000) {}
function shouldQuitAfterLastWindow() { return false; }
```

- [ ] **Step 1: pipe、background、quit の失敗テストを書く**

- spawn argv は `['engine']` で `--own-engine` を含まない。
- stdin は pipe で、renderer/helper へ渡さない。
- window-all-closed は tray の成否と OS に関係なく app.quit しない。
- explicit Quit は live session confirmation 後に stdin.end、最大5秒 wait、残れば kill。
- child unexpected exit は app shell を error とともに終了する。
- second-instance は hidden window も restore/focus する。

- [ ] **Step 2: current lifecycle test が落ちることを確認する**

Run: `npm test --prefix desktop`
Expected: FAIL

- [ ] **Step 3: engine spawn と entrance collection を繋ぎ替える**

`spawn(binary(), ["engine"], engineSpawnOptions())` とし、stdout の bootstrap URL は Electron 内だけで読む。stdin writer を `engine` object 以外へ expose しない。

- [ ] **Step 4: window close と app quit を分ける**

window close は window reference だけを片付ける。tray 作成失敗でも Electron/engine は残す。Dock、launcher、bare CLI、tray action、second-instance が同じ `openOrFocusWindow` を呼ぶ。

- [ ] **Step 5: graceful/forced child shutdown を実装する**

normal quit は ownership pipe を close し、exit event を最大5秒待つ。timeout 後のみ child process を強制終了する。session confirmation で cancel された quit は pipe を閉じない。

- [ ] **Step 6: Node tests と package syntax を通す**

Run: `npm test --prefix desktop`
Expected: PASS

Run: `node --check desktop/main.js && node --check desktop/lifecycle.js`
Expected: PASS

- [ ] **Step 7: Go regression、build、commit**

Run: `go test ./cmd/sshc ./internal/app`
Expected: PASS

Run: `go build ./...`
Expected: PASS

```bash
git add desktop/main.js desktop/entrance.js desktop/entrance.test.js desktop/reopen.js desktop/reopen.test.js desktop/lifecycle.js desktop/lifecycle.test.js desktop/package.json
git commit -m "feat: own the desktop engine through a lifetime pipe"
```

---

### Task 9: stable managed CLI と Linux launcher descriptor を実装する

**Files:**
- Replace: `desktop/link.js`
- Modify: `desktop/link.test.js`
- Create: `desktop/install-cli.js`
- Create: `desktop/install-cli.test.js`
- Create: `desktop/launcher.js`
- Create: `desktop/launcher.test.js`
- Modify: `desktop/main.js`
- Modify: `desktop/package.json`

**Interfaces:**

```js
async function installManagedCLI({ source, managed, publicPath, fs }) {}
async function recordLinuxLauncher({ appImage, descriptorPath, fs }) {}
```

- [ ] **Step 1: atomic copy と preservation の失敗テストを書く**

- bundled CLI を `~/.local/share/sshc/bin/sshc` へ temp + fsync + rename で copy する。
- public `~/.local/bin/sshc` は managed path への正しい symlink のときだけ更新可能。
- unrelated regular file、別 target symlink、broken unrelated symlink は上書きせず actionable warning を返す。
- AppImage mount 内 resource を public symlink target にしない。
- descriptor は absolute `APPIMAGE` だけを 0600 で atomic write する。
- `APPIMAGE` missing/relative は記録しない。

- [ ] **Step 2: current symlink implementation で落ちることを確認する**

Run: `node --test desktop/link.test.js desktop/install-cli.test.js desktop/launcher.test.js`
Expected: FAIL

- [ ] **Step 3: managed copy と safe public link を実装する**

managed directory は 0700、binary は 0700 とする。copy 元と managed file の digest/size を比較して同一なら不要な置換を避ける。rename 後に directory sync を可能な OS で行う。

- [ ] **Step 4: Linux descriptor を app startup に接続する**

packaged Linux かつ `APPIMAGE` が有効なときだけ `~/.ssh/sshc/desktop.json` を更新する。macOS では descriptor を作らず bundle id activation を使う。

- [ ] **Step 5: warning を UI と stderr に出す**

CLI public path を保護して install を skip した場合、Electron dialog または app 内 notification に exact path と手動解決手順を出す。warning に bootstrap token や vault state を含めない。

- [ ] **Step 6: tests、build、commit**

Run: `npm test --prefix desktop`
Expected: PASS

Run: `go test ./cmd/sshc`
Expected: PASS

Run: `go build ./...`
Expected: PASS

```bash
git add desktop/link.js desktop/link.test.js desktop/install-cli.js desktop/install-cli.test.js desktop/launcher.js desktop/launcher.test.js desktop/main.js desktop/package.json
git commit -m "feat: install a stable desktop-managed cli"
```

---

### Task 10: isolated process integration で common contract を固定する

**Files:**
- Create: `integration/engine_ownership_test.go`
- Create: `integration/vault_cli_test.go`
- Create: `integration/desktop_wait_test.go`
- Modify: `Makefile`
- Modify: `README.md`

**Interfaces:**

```go
type testProcess struct {
	Command *exec.Cmd
	Stdout  *bytes.Buffer
	Stderr  *bytes.Buffer
}
```

- [ ] **Step 1: isolated HOME の real-binary test harness を書く**

test ごとに `t.TempDir()` を HOME、SSH tree、desktop descriptor に使い、`go test` の helper process ではなく実際に build した `sshc` を起動する。production HOME の handoff/lock/process を参照しない。

- [ ] **Step 2: ownership process cases を追加する**

- bare `sshc` は lock を保持しない。
- headless は foreground、locked/missing announcement に `#bootstrap=` がない。
- headless 二台目は code 1。
- desktop ownership pipe EOF は child を終了する。
- desktop/headless race の winner は一台だけ。
- SIGKILL 後に OS lock が解放され、stale handoff を次 owner が置換する。

- [ ] **Step 3: Vault CLI process cases を追加する**

pseudo-terminal test helper を使って create/unlock/change の入力を与え、redirected stdin は拒否する。canary password を process args、environment dump、stdout/stderr、handoff、temp files から search する。lock 中も事前に確立した test session が残ることを server probe で確認する。

- [ ] **Step 4: desktop unlock wait process case を追加する**

fake desktop launcher が `sshc engine` を ownership pipe 付きで起動し、locked alias CLI が生きたまま待つこと、別 PTY の `vault unlock` 後に同じ CLI PID が接続段階へ進むこと、Ctrl-C が 130 になることを検査する。

- [ ] **Step 5: common README を実装済み範囲に合わせる**

desktop/headless distinction、Vault CLI、bare activation、macOS/Linux launcher、Linux no-display path、AppImage relocation を記載する。Windows support claim と release matrix は Windows/CI plans 完了まで「実装中」とし、完了を先取りしない。

- [ ] **Step 6: full common verification を通す**

Run: `go test ./...`
Expected: PASS

Run: `go test -race ./...`
Expected: PASS on macOS/Linux

Run: `go vet ./...`
Expected: PASS

Run: `go build ./...`
Expected: PASS

Run: `npm test --prefix desktop`
Expected: PASS

Run: `npm test --prefix web`
Expected: PASS

- [ ] **Step 7: generated UI と diff hygiene を確認する**

Run: `npm run build --prefix web && git status --short internal/ui/dist`
Expected: generated bundle が source と一致し、必要な差分だけが表示される

Run: `rg -n -- '--own-engine|run the engine here|watchParent|Getppid' README.md cmd desktop internal docs/superpowers/specs/2026-08-15-explicit-desktop-headless-windows-design.md`
Expected: historical explanation を除き実装・help・README に旧 behavior が残らない

- [ ] **Step 8: commit**

```bash
git add integration Makefile README.md internal/ui/dist
git commit -m "test: verify explicit engine ownership end to end"
```

---

## Completion Gate

- [ ] bare `sshc` が engine を起動しないことを real process で確認した。
- [ ] desktop engine は Electron ownership pipe EOF で終了する。
- [ ] headless は foreground で、秘密を含まない解錠案内を出す。
- [ ] desktop/headless の Go engine が同時に一台を超えない。
- [ ] desktop locked connection は同じ CLI で待ち、unlock 後に接続する。
- [ ] headless locked connection は待たず Vault CLI を案内する。
- [ ] missing vault を unlocked と扱わず、desktop は create を待ち headless は create を案内する。
- [ ] `vault lock` が既存 session を終了しない。
- [ ] old command/handoff/protocol fallback が無い。
- [ ] macOS/Linux の launcher と stable managed CLI が package 外の永続 path を使う。
- [ ] Windows 固有の未実装事項を macOS/Linux fallback で隠していない。
