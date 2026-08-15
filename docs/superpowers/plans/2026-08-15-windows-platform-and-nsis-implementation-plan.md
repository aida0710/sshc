# Windows Platform and NSIS Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 共通の explicit owner/protocol 実装を Windows の実動作へ拡張し、保存データの安全性、単一 engine、OpenSSH agent、ConPTY local shell、desktop activation、per-user NSIS installer を native Windows で成立させる。

**Architecture:** portable interface は共通 plan で固定し、Windows は build-tagged adapter を提供する。filesystem は reparse point を拒否し restricted DACL と atomic replace を担保する。engine lock は `LockFileEx`、desktop discovery は HKCU、agent は OpenSSH named pipe、local shell は ConPTY + kill-on-close Job Object を用いる。Electron builder は app と同一版の Go binary を `resources\cli` に同梱し、NSIS が user PATH と launcher registry を exact entry 単位で管理する。

**Tech Stack:** Go 1.26 / `golang.org/x/sys/windows` / `golang.org/x/crypto/ssh/agent` / Electron 43 / electron-builder NSIS / PowerShell

**Spec:** `docs/superpowers/specs/2026-08-15-explicit-desktop-headless-windows-design.md`

## Global Constraints

- `2026-08-15-explicit-engine-owners-and-vault-cli-implementation-plan.md` を先に完了する。
- Windows の安全検査を no-op、Unix path、runtime branch で代用しない。
- Windows 10 version 1809 以降を対象とする。
- installer は per-user、non-elevated とし、machine PATH/HKLM を変更しない。
- connection transport は in-process SSH のままにし、`ssh.exe` を起動しない。
- `ssh-keygen.exe` と shell は trusted absolute path だけから選び、PATH search しない。
- private state は current user、SYSTEM、Administrators の restricted DACL を持つ。
- package をグローバルまたはユーザー環境へインストールしない。repository の既存 `x/sys` で実装する。
- Windows native behavior は Windows runner で検査し、macOS からの cross-build だけで完了扱いにしない。
- x64 と arm64 は別 artifact とし、各 architecture の実行証拠が揃うまで support claim を付けない。
- 各タスクは Windows cross-build と対象 package test を通し、native-only assertion は未実行と明記してから commit する。

---

### Task 1: portable filesystem core と OS adapter の境界を作る

**Files:**
- Modify: `internal/storage/filesystem.go`
- Create: `internal/storage/filesystem_unix.go`
- Create: `internal/storage/filesystem_windows.go`
- Modify: `internal/storage/filesystem_test.go`
- Create: `internal/storage/filesystem_windows_test.go`
- Create: `internal/storage/filesystem_unix_test.go`

**Interfaces:**

```go
type nativeFileSystem struct{}

func openRegularNoFollow(path string) (*os.File, error)
func replaceFile(oldPath, newPath string) error
func syncDirectory(path string) error
func restrictPrivatePath(path string, directory bool) error
```

- [ ] **Step 1: common behavior と Windows compile の失敗テストを書く**

common test は regular file size limit、non-regular refusal、temp cleanup、replace semantics を `OSFileSystem` に対して検査する。Windows-only test は symlink、junction、directory reparse point を作り、`ReadFile` が `ErrSymlinkPath` を返すことを検査する。

- [ ] **Step 2: 現在の `syscall.O_NOFOLLOW` で cross-build が落ちることを確認する**

Run: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/sshc-storage.test.exe ./internal/storage`
Expected: FAIL（`syscall.O_NOFOLLOW` が Windows で未定義）

- [ ] **Step 3: common logic から Unix syscall を分離する**

`filesystem.go` は interface、limit、read-after-open、writeAndFlush だけを持つ。Unix file に `O_NOFOLLOW` open、`os.Rename`、directory fsync を移す。既存 Unix safety test の意味を変えない。

- [ ] **Step 4: Windows no-follow open を実装する**

`windows.CreateFile` に `FILE_READ_DATA|FILE_READ_ATTRIBUTES`、share read/write/delete、`OPEN_EXISTING`、`FILE_FLAG_OPEN_REPARSE_POINT` を渡す。`GetFileInformationByHandleEx(FileAttributeTagInfo)` で `FILE_ATTRIBUTE_REPARSE_POINT` を拒否し、regular file だけ `os.NewFile` に移す。handle error path は必ず close する。

- [ ] **Step 5: Windows atomic replace と directory sync contract を実装する**

`MoveFileEx(MOVEFILE_REPLACE_EXISTING|MOVEFILE_WRITE_THROUGH)` を使う。Windows に Unix directory fsync が無いことを `syncDirectory` の contract comment に明記し、file flush + write-through replace を durability boundary とする。`Rename` の common call site は `replaceFile` を使う。

- [ ] **Step 6: cross-build と Unix regression を通す**

Run: `go test ./internal/storage`
Expected: PASS

Run: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/sshc-storage.test.exe ./internal/storage`
Expected: PASS

Run: `GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/sshc-storage-arm64.test.exe ./internal/storage`
Expected: PASS

- [ ] **Step 7: commit**

```bash
git add internal/storage/filesystem.go internal/storage/filesystem_unix.go internal/storage/filesystem_windows.go internal/storage/filesystem_test.go internal/storage/filesystem_unix_test.go internal/storage/filesystem_windows_test.go
git commit -m "feat: add safe windows filesystem primitives"
```

---

### Task 2: private Windows DACL を state write 全体へ適用する

**Files:**
- Create: `internal/platform/windowsacl/acl_windows.go`
- Create: `internal/platform/windowsacl/acl_windows_test.go`
- Create: `internal/platform/windowsacl/acl_other.go`
- Modify: `internal/storage/filesystem_windows.go`
- Modify: `internal/handoff/handoff.go`
- Create: `internal/handoff/permissions_windows_test.go`
- Modify: `internal/secret/vault.go`
- Modify: `internal/storage/transaction.go`

**Interfaces:**

```go
package windowsacl

func RestrictFile(path string) error
func RestrictDirectory(path string) error
func IsRestrictedToCurrentUser(path string) (bool, error)
```

- [ ] **Step 1: inherited permissive ACL を締める native failure test を書く**

Windows temp parent に Users read permission を与え、その下へ state directory/file を作る。適用後の DACL を `GetNamedSecurityInfo` で読み、current user、SYSTEM、Administrators 以外の allow ACE が無いこと、inheritance protected bit が立つことを検査する。

- [ ] **Step 2: native Windows で test が未定義により落ちることを確認する**

Run on Windows: `go test ./internal/platform/windowsacl ./internal/handoff ./internal/storage`
Expected: FAIL

- [ ] **Step 3: current user SID を含む protected DACL を実装する**

process token の user SID を `currentUserSID` として読み、`"D:P(A;;FA;;;" + currentUserSID + ")(A;;FA;;;SY)(A;;FA;;;BA)"` を `SecurityDescriptorFromString` で構成し、`SetNamedSecurityInfo` で DACL を設定する。SID string は log へ不要に出さない。

- [ ] **Step 4: 全 private write point へ適用する**

storage temp、destination、state directory、handoff temp/final、vault、journal、backup、sync settings の write path を列挙して adapter を通す。存在する file list は実装開始時に次で固定する。

Run: `rg -n 'WriteFile|CreateTemp|OpenFile|Rename\(' internal cmd`
Expected: 各 private writer が storage/handoff adapter 経由か、非機密である理由を review note に持つ

- [ ] **Step 5: native ACL tests と cross-build を通す**

Run on Windows: `go test ./internal/platform/windowsacl ./internal/storage ./internal/handoff ./internal/secret`
Expected: PASS

Run: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`
Expected: PASS

- [ ] **Step 6: commit**

```bash
git add internal/platform/windowsacl internal/storage internal/handoff internal/secret
git commit -m "feat: restrict private windows state with dacl"
```

---

### Task 3: Windows engine lock adapter を実装する

**Files:**
- Create: `internal/enginelock/lock.go`
- Create: `internal/enginelock/lock_unix.go`
- Create: `internal/enginelock/lock_windows.go`
- Create: `internal/enginelock/lock_unix_test.go`
- Create: `internal/enginelock/lock_windows_test.go`
- Modify: `cmd/sshc/lock.go`
- Delete: `cmd/sshc/lock_unix.go`
- Delete: `cmd/sshc/lock_other.go`

**Interfaces:**

```go
var ErrRunning = errors.New("an sshc engine is already running")

func Acquire(path string) (release func() error, err error)
```

- [ ] **Step 1: real process lock test を Windows build tag で書く**

parent test process が lock handle を保持し、child helper process が同じ file で `errEngineRunning` になること、parent exit 後は別 child が取得できることを検査する。同一 process 内の二重取得だけで済ませない。

- [ ] **Step 2: no-op `lock_other.go` のため test が落ちることを確認する**

Run on Windows: `go test ./internal/enginelock -run 'TestWindowsEngineLock'`
Expected: FAIL

- [ ] **Step 3: `LockFileEx` lock を実装する**

private directory/file を作り、`LOCKFILE_EXCLUSIVE_LOCK|LOCKFILE_FAIL_IMMEDIATELY` で 1 byte を lock する。sharing violation/lock violation を `errEngineRunning` に写像する。release は `UnlockFileEx` 後に handle close し、複数回呼ばれても panic しない。

- [ ] **Step 4: cmd lock wrapper を共通 package へ接続する**

既存 Unix lock implementation も `internal/enginelock` へ移し、`cmd/sshc/lock.go` は `enginelock.ErrRunning` を既存 `errEngineRunning` に写像する薄い wrapper にする。Windows の no-op fallback は削除する。console cancellation は全 adapter が揃う Task 6 で cmd package に接続する。

- [ ] **Step 5: native race と abnormal-exit tests を通す**

Run on Windows: `go test -race ./internal/enginelock -run 'TestWindowsEngineLock'`
Expected: PASS（toolchain が Windows race を提供しない場合は理由を CI plan の matrix output に記録し、通常 test は必須）

Run: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/sshc-enginelock.test.exe ./internal/enginelock`
Expected: PASS

- [ ] **Step 6: commit**

```bash
git add internal/enginelock cmd/sshc/lock.go cmd/sshc/lock_unix.go cmd/sshc/lock_other.go
git commit -m "feat: enforce one engine with lockfileex"
```

---

### Task 4: Windows trusted toolchain と shell policy を実装する

**Files:**
- Create: `internal/platform/windows/toolchain.go`
- Create: `internal/platform/windows/toolchain_test.go`
- Create: `internal/platform/shell_unix.go`
- Create: `internal/platform/shell_windows.go`
- Modify: `internal/platform/shell.go`
- Create: `internal/platform/shell_windows_test.go`

**Interfaces:**

```go
package windows

func NewToolchain(windowsDirectory string) platform.Toolchain

func LoginShell(lookup func(string) (string, bool), stat func(string) error) (string, error)
func LoginArguments(shell string) []string
```

- [ ] **Step 1: PATH poisoning refusal の失敗テストを書く**

fake PATH の `ssh-keygen.exe` と `pwsh.exe` を見つけないこと、次の absolute candidate order を検査する。

1. PowerShell 7 の registered/known Program Files absolute path
2. `%WINDIR%\System32\WindowsPowerShell\v1.0\powershell.exe`
3. absolute existing `%ComSpec%`

`ssh-keygen.exe` は `%WINDIR%\System32\OpenSSH\ssh-keygen.exe` を最初にし、存在しなければ unavailable とする。

- [ ] **Step 2: current Unix shell assumptions で cross-build test が落ちることを確認する**

Run on Windows: `go test ./internal/platform ./internal/platform/windows`
Expected: FAIL（Windows shell が trusted candidate を選べず、Windows toolchain package が未定義）

- [ ] **Step 3: shell policy と OS-specific execution fields を分離する**

common `Command` data type は維持し、Unix の `SHELL`、login argv0、fallback paths を `shell_unix.go` へ移す。Windows は `-NoLogo` を arguments にし、Unix の `-shellname` argv0 を作らない。

- [ ] **Step 4: Windows toolchain と shell command construction を追加する**

toolchain と shell resolver は production constructor を提供する。Windows key agent と ConPTY がまだ未完成なので、このタスクでは `cmd/sshc` に dummy/no-op wiring を追加しない。全 production wiring は Task 6 で一度に接続する。

- [ ] **Step 5: tests と cross-build を通す**

Run: `go test ./internal/platform/...`
Expected: PASS on current OS

Run: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/sshc-platform.test.exe ./internal/platform`
Expected: PASS

Run: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/sshc-windows-platform.test.exe ./internal/platform/windows`
Expected: PASS

- [ ] **Step 6: commit**

```bash
git add internal/platform/windows internal/platform/shell.go internal/platform/shell_unix.go internal/platform/shell_windows.go internal/platform/shell_windows_test.go
git commit -m "feat: add trusted windows platform wiring"
```

---

### Task 5: Windows OpenSSH named-pipe agent adapter を実装する

**Files:**
- Create: `internal/platform/windowspipe/conn_windows.go`
- Create: `internal/platform/windowspipe/conn_windows_test.go`
- Create: `internal/platform/windowspipe/conn_other.go`
- Create: `internal/keys/agent_windows.go`
- Create: `internal/keys/agent_windows_test.go`
- Create: `internal/keys/agent_unix.go`
- Modify: `internal/keys/agent.go`

**Interfaces:**

```go
package windowspipe

func DialContext(ctx context.Context, path string) (net.Conn, error)

package keys

func NewWindowsAgent() platform.KeyAgent
```

- [ ] **Step 1: named-pipe framing と cancellation の native failure test を書く**

test named-pipe server は `x/crypto/ssh/agent` protocol の request/response framing を echo/fake し、client `List` が identity を decode できることを検査する。server が応答しない場合に context deadline で read/write が解除されること、Close が pending I/O を解除することも検査する。

- [ ] **Step 2: Unix-only `net.Dialer{Network: unix}` で Windows test が落ちることを確認する**

Run on Windows: `go test ./internal/keys ./internal/platform/windowspipe`
Expected: FAIL

- [ ] **Step 3: x/sys-only named-pipe `net.Conn` を実装する**

`kernel32.dll` の `WaitNamedPipeW` を `windows.NewLazySystemDLL` で解決し、`windows.CreateFile` で `\\.\pipe\openssh-ssh-agent` を開く。overlapped `ReadFile`/`WriteFile` と `CancelIoEx` で context/deadline を実装する。`LocalAddr`/`RemoteAddr` は credential を含まない fixed pipe address を返す。handle は finalizer に頼らず Close する。

- [ ] **Step 4: agent construction を OS 別にする**

Unix は `SSH_AUTH_SOCK`、Windows は fixed named pipe を使う。Windows で environment variable や PATH から agent endpoint を差し替えない。agent unavailable は鍵/パスワードによる他の authentication を妨げない。

- [ ] **Step 5: protocol operation tests を通す**

List/Add/Remove が既存 fake agent server と Windows named pipe transport の両方で通ること、passphrase canary が pipe address/log/error に出ないことを確認する。

Run on Windows: `go test -race ./internal/keys ./internal/platform/windowspipe`
Expected: PASS（race availability の扱いは Task 3 と同じ）

Run: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/sshc-keys.test.exe ./internal/keys`
Expected: PASS

- [ ] **Step 6: commit**

```bash
git add internal/platform/windowspipe internal/keys/agent.go internal/keys/agent_unix.go internal/keys/agent_windows.go internal/keys/agent_windows_test.go
git commit -m "feat: speak to the windows openssh agent pipe"
```

---

### Task 6: ConPTY process、signals、production wiring を実装する

**Files:**
- Create: `internal/terminal/pty_windows.go`
- Create: `internal/terminal/pty_windows_test.go`
- Modify: `internal/terminal/terminal.go`
- Modify: `internal/terminal/pty_test.go`
- Create: `cmd/sshc/signals_windows.go`
- Create: `cmd/sshc/signals_windows_test.go`
- Create: `cmd/sshc/wiring_windows.go`
- Modify: `cmd/sshc/wiring.go`
- Modify: `cmd/sshc/engine_test.go`

**Interfaces:**

```go
type WindowsStarter struct{}

func NewStarter() Starter
func (WindowsStarter) Start(command Command, size Size) (Process, error)
```

`Process.Hangup` の interface comment は OS-neutral な「session process tree の終了要求」に改める。Unix implementation は SIGHUP、Windows implementation は Job Object close を使う。

- [ ] **Step 1: native ConPTY behavior の失敗テストを書く**

- PowerShell を起動し、marker を Write、Read できる。
- initial size と `ResizePseudoConsole` 後の size が console query output に反映される。
- child が descendant process を起動し、Hangup/Close 後に両方が消える。
- Wait は normal exit code と forced close を区別する。
- invalid size、missing absolute executable、relative path を拒否する。

- [ ] **Step 2: `NewStarter` 不在で Windows test が落ちることを確認する**

Run on Windows: `go test ./internal/terminal -run 'TestWindows'`
Expected: FAIL

- [ ] **Step 3: ConPTY pipes と pseudo console を構築する**

anonymous pipe pair を二組作り、`CreatePseudoConsole` に input/output handle を渡す。`STARTUPINFOEX` の `PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE` を構成し、absolute command path と quoted arguments から `CreateProcess` する。継承 handle list を最小化する。

- [ ] **Step 4: Job Object lifecycle を実装する**

`CreateJobObject` と `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` を設定し、child process を assign する。Hangup は job handle を close し、Close は ConPTY と pipe handles を一度だけ close する。Wait と Close の競合を `sync.Once` と channel で収束させる。

- [ ] **Step 5: read/write/deadline と resize を実装する**

Read/Write は pipe file を用い、Resize は validated `terminal.Size` を `windows.Coord` へ変換する。ConPTY unsupported error は Windows 10 1809 requirement を user-facing message にする。

- [ ] **Step 6: Windows signals と production wiring を接続する**

`os.Interrupt` と `syscall.SIGBREAK` を signal context に登録し、Ctrl-C/Ctrl-Break helper process で public exit 130 を検査する。`wiring_windows.go` は trusted Windows toolchain、named-pipe key agent、ConPTY starter を production dependencies に渡す。Electron desktop では ownership pipe EOF が primary cancellation のままである。

- [ ] **Step 7: native terminal/engine tests と full Windows build を通す**

Run on Windows: `go test -race ./internal/terminal ./internal/httpserver ./cmd/sshc -run 'TestWindows|TestEngineSignal|TestRunEngine'`
Expected: PASS（race availability の扱いは Task 3 と同じ）

Run: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/sshc-terminal.test.exe ./internal/terminal`
Expected: PASS

Run: `GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/sshc-terminal-arm64.test.exe ./internal/terminal`
Expected: PASS

Run: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`
Expected: PASS

Run: `GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build ./...`
Expected: PASS

- [ ] **Step 8: commit**

```bash
git add internal/terminal/terminal.go internal/terminal/pty_windows.go internal/terminal/pty_windows_test.go internal/terminal/pty_test.go cmd/sshc/signals_windows.go cmd/sshc/signals_windows_test.go cmd/sshc/wiring_windows.go cmd/sshc/wiring.go cmd/sshc/engine_test.go
git commit -m "feat: run local windows shells through conpty"
```

---

### Task 7: HKCU desktop launcher と Windows activation を実装する

**Files:**
- Create: `cmd/sshc/launch_windows.go`
- Create: `cmd/sshc/launch_windows_test.go`
- Modify: `cmd/sshc/launch_unsupported.go`
- Create: `internal/platform/windowsregistry/launcher_windows.go`
- Create: `internal/platform/windowsregistry/launcher_windows_test.go`
- Create: `internal/platform/windowsregistry/launcher_other.go`

**Interfaces:**

```go
const LauncherKey = `Software\sshc\Desktop`
const LauncherValue = "Executable"

func ReadDesktopExecutable() (string, error)
func RegisterDesktopExecutable(path string) error
func RemoveDesktopExecutable(expected string) error
```

- [ ] **Step 1: registry/path validation の native failure test を書く**

- missing key は installer/open app guidance。
- relative path、directory、missing file、wrong basename/type は拒否。
- valid absolute Electron executable は shell なしで起動。
- remove は stored value が expected path と一致する場合だけ削除。
- live headless owner の bare activation は registry executable を起動しない。

- [ ] **Step 2: Windows launcher 未定義で test が落ちることを確認する**

Run on Windows: `go test ./cmd/sshc ./internal/platform/windowsregistry -run 'TestWindows|TestLauncher'`
Expected: FAIL

- [ ] **Step 3: HKCU adapter と direct exec を実装する**

`golang.org/x/sys/windows/registry` の CURRENT_USER を使い、read access と write access を分離する。CLI は registered absolute path を `exec.CommandContext(path)` で実行し、`cmd.exe /c`、PowerShell、PATH search を使わない。

- [ ] **Step 4: desktop second-instance behavior を package-independent test で確認する**

fake registered executable が activation count を記録する test を追加し、bare CLI と locked desktop focus が同じ launcher path を一回だけ呼ぶことを検査する。

- [ ] **Step 5: native tests と build を通す**

Run on Windows: `go test ./cmd/sshc ./internal/platform/windowsregistry`
Expected: PASS

Run: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`
Expected: PASS

- [ ] **Step 6: commit**

```bash
git add cmd/sshc/launch_windows.go cmd/sshc/launch_windows_test.go cmd/sshc/launch_unsupported.go internal/platform/windowsregistry
git commit -m "feat: activate the registered windows desktop"
```

---

### Task 8: Electron Windows resource layout と NSIS per-user installer を作る

**Files:**
- Modify: `desktop/package.json`
- Modify: `desktop/package-lock.json`
- Modify: `desktop/main.js`
- Create: `desktop/build/installer.nsh`
- Create: `desktop/installer.js`
- Create: `desktop/installer.test.js`
- Modify: `desktop/install-cli.js`
- Modify: `desktop/install-cli.test.js`
- Modify: `Makefile`

**Interfaces:**

```js
function engineBinary({ platform, resourcesPath }) {
  return platform === "win32"
    ? path.join(resourcesPath, "cli", "sshc.exe")
    : path.join(resourcesPath, "sshc");
}
```

- [ ] **Step 1: layout と installer mutation の失敗テストを書く**

package config を読み、Windows artifact が次を満たすことを test する。

- Electron: `%LOCALAPPDATA%\Programs\sshc\sshc.exe`
- Go CLI: `resources\cli\sshc.exe`
- perMachine false、requestedExecutionLevel user、oneClick policy 明示。
- PATH add は CLI directory exact normalized entry を重複なく追加。
- uninstall は exact entry だけ削除し、他 entry と近似 prefix を保持。
- HKCU launcher value は Electron executable、uninstall は expected match のときだけ削除。

- [ ] **Step 2: current package config に Windows target がなく落ちることを確認する**

Run: `node --test desktop/installer.test.js`
Expected: FAIL

- [ ] **Step 3: package resource と target を設定する**

`extraResources` を OS ごとに正しい executable suffix/location へ写す。Windows `nsis` target を x64/arm64 で追加し、artifact name に version、os、arch を含める。`dist` script は `--mac --linux --win` を一つで実行せず、native workflow が target ごとの script を呼べるよう `dist:mac`、`dist:linux`、`dist:win` に分ける。

- [ ] **Step 4: NSIS user PATH と HKCU macro を実装する**

installer include は `HKCU\Environment\Path` を semicolon-aware に parse し、case-insensitive normalized exact match で CLI dir を一件だけ追加する。uninstall は同じ exact entry だけを削除する。environment change broadcast を送り、HKCU sshc-owned launcher key を登録・削除する。HKLM と machine PATH へ触れない。

- [ ] **Step 5: Windows では desktop が CLI を自己 copy/symlink しないよう分岐する**

NSIS layout が安定 path を担うため、Electron startup は Windows で Unix managed-copy installer を呼ばない。engine path は `resources\cli\sshc.exe` を absolute path で使う。

- [ ] **Step 6: package config tests と unpacked build を通す**

Run: `npm test --prefix desktop`
Expected: PASS

Run on Windows: `npm run dist:win --prefix desktop -- --dir`
Expected: unpacked app に `resources\cli\sshc.exe` があり起動可能

- [ ] **Step 7: Makefile の Windows bundles を作る**

`GOOS=windows GOARCH=amd64/arm64 CGO_ENABLED=0` で `.exe` を `desktop/resources/win32-x64/sshc.exe` と `win32-arm64/sshc.exe` へ build する。clean target は repository 生成物だけを exact path で除く。

- [ ] **Step 8: commit**

```bash
git add desktop/package.json desktop/package-lock.json desktop/main.js desktop/build/installer.nsh desktop/installer.js desktop/installer.test.js desktop/install-cli.js desktop/install-cli.test.js Makefile
git commit -m "feat: package sshc as a per-user windows app"
```

---

### Task 9: native Windows package smoke suite を作る

**Files:**
- Create: `scripts/windows/package-smoke.ps1`
- Create: `scripts/windows/assert-private-acl.ps1`
- Create: `scripts/windows/process-tree.ps1`
- Create: `integration/windows_process_test.go`
- Modify: `desktop/package.json`
- Modify: `README.md`

**Interfaces:**

```powershell
param(
  [Parameter(Mandatory = $true)][string]$Installer,
  [Parameter(Mandatory = $true)][string]$Architecture,
  [Parameter(Mandatory = $true)][string]$WorkRoot
)
```

- [ ] **Step 1: installer smoke script を assertion-first で書く**

isolated local user context と unique WorkRoot を使い、silent install → file layout → user PATH → HKCU launcher → app launch → CLI status → uninstall → exact cleanup の順に検査する。失敗時は secret/handoff body を dump しない。

- [ ] **Step 2: engine/process native integration を追加する**

- bare CLI が engine lock を持たない。
- headless と desktop が排他。
- Electron process 強制終了後に Go engine が ownership EOF で終了。
- window close 後は Electron/engine/session が残る。
- app Quit 後は engine/ConPTY descendants が残らない。
- LockFileEx は abnormal engine death 後に解放される。

- [ ] **Step 3: filesystem/ACL/agent/ConPTY smoke を統合する**

reparse refusal、handoff/vault DACL、OpenSSH Authentication Agent service が利用可能な場合の named-pipe list、ConPTY start/resize/descendant cleanup を実行する。agent service unavailable は agent test を skipped evidence とし、他項目を success に偽装しない。

- [ ] **Step 4: PowerShell から desktop activation と clipboard を検査する**

bare `sshc` で existing window が focus され、selection-copy と right-click-paste の enable/disable が packaged Electron と system clipboard 間で動くことを Playwright/Electron harness から検査する。clipboard contents は canary の一致だけを assert し log しない。

- [ ] **Step 5: x64 native suite を通す**

Run on Windows x64:

```powershell
npm run dist:win --prefix desktop
pwsh -File scripts/windows/package-smoke.ps1 -Installer $env:SSHC_NSIS_X64 -Architecture x64 -WorkRoot $env:RUNNER_TEMP\sshc-smoke
go test ./...
go vet ./...
```

Expected: PASS

- [ ] **Step 6: arm64 native/real-machine evidence を取る**

arm64 runner または real Windows arm64 machine で同じ smoke suite を実行する。利用できない場合は artifact を `unverified` とし、supported matrix に含めない。

- [ ] **Step 7: README の Windows section を実証済み範囲だけ更新する**

NSIS per-user、user PATH、Startup Apps、SmartScreen、headless PowerShell example、ConPTY requirement、verified architecture を記載する。unsigned installer の警告を signing 済みのように表現しない。

- [ ] **Step 8: commit**

```bash
git add scripts/windows integration/windows_process_test.go desktop/package.json README.md
git commit -m "test: verify the native windows package"
```

---

## Completion Gate

- [ ] Windows amd64/arm64 cross-build が共に通る。
- [ ] Windows native test で reparse point refusal と restricted DACL を確認した。
- [ ] real-process `LockFileEx` test で一台だけの engine を確認した。
- [ ] Ctrl-C/Ctrl-Break と desktop ownership EOF が graceful shutdown に入る。
- [ ] OpenSSH named pipe agent が protocol operation と deadline を満たす。
- [ ] ConPTY が start/read/write/resize/exit と descendant cleanup を満たす。
- [ ] launcher は HKCU absolute executable だけを直接起動する。
- [ ] NSIS は non-admin で user PATH の exact entry と sshc-owned registry key だけを変更する。
- [ ] install/uninstall/package smoke が Windows x64 で通る。
- [ ] arm64 を support と書く場合は arm64 実行証拠がある。
- [ ] macOS/Linux の既存 safety test を Windows 対応のために弱めていない。
