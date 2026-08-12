# Stable Install and Service Rebind Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `make install` atomically install sshc at a stable per-user path and rebind only an already-enabled login service to that exact binary.

**Architecture:** A reserved `sshc service refresh|disable` maintenance command owns service lifecycle through the existing platform LoginItem implementations. The Makefile owns only binary staging and sequencing: it atomically replaces the installed file, asks that installed file to refresh an enabled service, and refuses to remove the file until a freshly built binary has disabled the service.

**Tech Stack:** Go 1.26, platform-specific Go build files, launchd, systemd user services, POSIX Make recipes, Go integration tests.

## Global Constraints

- Keep the stable default path at `$(HOME)/.local/bin/sshc`; do not add sudo, root installation, Homebrew, deb, or rpm packaging.
- `make install` must not enable login startup when it is currently disabled.
- Refresh must register only the running maintenance binary resolved through `os.Executable()`; it must not accept an arbitrary program path from argv.
- Restarting an enabled service intentionally locks the vault and disconnects existing Web sessions.
- Service maintenance must not open a browser, start an SSH connection, print a bootstrap URL, or expose a secret.
- `make uninstall` must retain the installed binary if service disable fails.
- Do not add or update Go or npm dependencies or lockfiles.
- Verify install behavior with isolated `HOME` and `INSTALL_DIR`; do not restart the user's live service during automated verification.

---

## File map

- Create `cmd/sshc/service.go`: platform-neutral maintenance command parsing and behavior.
- Create `cmd/sshc/service_test.go`: command routing, no-op, refresh, disable, and error contracts.
- Create `cmd/sshc/service_darwin.go`: construct the existing macOS LoginItem for maintenance.
- Create `cmd/sshc/service_linux.go`: construct the Linux LoginItem and distinguish an absent systemd installation from a stranded unit.
- Create `cmd/sshc/service_linux_test.go`: Linux-only controller construction cases.
- Modify `cmd/sshc/main.go`: route `service` before alias handling and document it in usage.
- Modify `cmd/sshc/connect.go` and `cmd/sshc/connect_test.go`: reserve `service` as a command word.
- Modify `internal/platform/linux/loginitem.go`: expose registration errors and explicitly restart after rewriting a unit.
- Modify `internal/platform/linux/loginitem_test.go`: protect the new systemctl sequence and registration status.
- Create `internal/acceptance/install_test.go`: execute the Makefile's isolated install/uninstall primitives against fake binaries.
- Modify `Makefile`: atomic replacement, refresh sequencing, guarded uninstall, and test seams.
- Modify `README.md`: explain stable installation, refresh/no-enable behavior, vault locking, and the new maintenance commands.

---

### Task 1: Maintenance CLI contract

**Files:**
- Create: `cmd/sshc/service.go`
- Create: `cmd/sshc/service_test.go`
- Modify: `cmd/sshc/main.go`
- Modify: `cmd/sshc/connect.go`
- Modify: `cmd/sshc/connect_test.go`

**Interfaces:**
- Produces: `const ServiceSubcommand = "service"`
- Produces: `type serviceLoginItem interface { Enabled() bool; Enable(context.Context, string) error; Disable(context.Context) error }`
- Produces: `func serviceInvocation(argv []string) bool`
- Produces: `func runService(context.Context, []string, serviceLoginItem, func() (string, error), io.Writer, io.Writer) int`
- Produces temporarily: `newServiceLoginItem(home string) (serviceLoginItem, error)` using the existing platform assembly, replaced by OS-specific files in Task 2.

- [ ] **Step 1: Write failing command tests**

  Add table-driven tests proving that `service` is reserved even with missing or extra action arguments, then add behavioral tests with a recording LoginItem:

  ```go
  func TestServiceRefreshDoesNotEnableADisabledService(t *testing.T) {
      item := &recordingServiceLoginItem{}
      resolverCalled := false
      var stdout, stderr bytes.Buffer
      code := runService(context.Background(), []string{"refresh"}, item, func() (string, error) {
          resolverCalled = true
          return "/tmp/sshc", nil
      }, &stdout, &stderr)
      if code != 0 || resolverCalled || item.enableCalls != 0 {
          t.Fatalf("code=%d resolver=%v enable=%d", code, resolverCalled, item.enableCalls)
      }
  }

  func TestServiceRefreshRebindsAnEnabledServiceToThisExecutable(t *testing.T) {
      item := &recordingServiceLoginItem{enabled: true}
      code := runService(context.Background(), []string{"refresh"}, item,
          func() (string, error) { return "/Users/tester/.local/bin/sshc", nil },
          io.Discard, io.Discard)
      if code != 0 || item.enabledProgram != "/Users/tester/.local/bin/sshc" {
          t.Fatalf("code=%d program=%q", code, item.enabledProgram)
      }
  }
  ```

  Cover `disable` no-op and success, resolver failure, Enable/Disable failure, nil controller no-op, unknown/missing action returning exit 2, and usage output naming both actions. Assertions must be on return codes, recorded platform operations, and output secrecy—not on private helper calls.

- [ ] **Step 2: Run the focused tests and verify RED**

  Run: `go test ./cmd/sshc -run 'Test(Service|WhatCountsAsAConnectInvocation)' -count=1`

  Expected: compile failures for `runService`, `ServiceSubcommand`, and `serviceInvocation`, plus the current alias-routing test accepting `service`.

- [ ] **Step 3: Implement the minimum maintenance command**

  In `service.go`, validate exactly one action. For `refresh`, return success without resolving the executable when the controller is nil or disabled; otherwise resolve and require an absolute executable path before calling `Enable`. For `disable`, return success when nil or disabled and otherwise call `Disable`. Use concise one-line status and error output; never print a URL or environment.

  Route `serviceInvocation(os.Args)` in `main.go` after askpass and before `open`/alias handling:

  ```go
  if serviceInvocation(os.Args) {
      home, err := os.UserHomeDir()
      // report and exit on errors
      item, err := newServiceLoginItem(home)
      // report and exit on errors
      os.Exit(runService(context.Background(), os.Args[2:], item,
          os.Executable, os.Stdout, os.Stderr))
  }
  ```

  Add `ServiceSubcommand` to `connectInvocation`'s reserved words and list these usage lines:

  ```text
  sshc service refresh  rebind an enabled login service to this binary
  sshc service disable  stop and remove the login service
  ```

- [ ] **Step 4: Run the focused tests and verify GREEN**

  Run: `go test ./cmd/sshc -run 'Test(Service|WhatCountsAsAConnectInvocation)' -count=1`

  Expected: PASS with no browser, SSH, or server process started.

- [ ] **Step 5: Commit Task 1**

  ```bash
  git add cmd/sshc/service.go cmd/sshc/service_test.go cmd/sshc/main.go cmd/sshc/connect.go cmd/sshc/connect_test.go
  git commit -m "feat: add login service maintenance command"
  ```

### Task 2: Platform service refresh behavior

**Files:**
- Create: `cmd/sshc/service_darwin.go`
- Create: `cmd/sshc/service_linux.go`
- Create: `cmd/sshc/service_linux_test.go`
- Modify: `internal/platform/linux/loginitem.go`
- Modify: `internal/platform/linux/loginitem_test.go`

**Interfaces:**
- Consumes: `serviceLoginItem` from Task 1.
- Produces: `func newServiceLoginItem(home string) (serviceLoginItem, error)` on darwin and linux.
- Produces: `func (LoginItem) Registered() (bool, error)` in the Linux platform package.
- Produces for Linux tests: `func newLinuxServiceLoginItem(home string, stat func(string) (os.FileInfo, error), runner platform.OutputRunner) (serviceLoginItem, error)`.

- [ ] **Step 1: Write failing Linux LoginItem tests**

  Change the hand-derived expected systemctl sequence to:

  ```go
  want := []platform.Command{
      {Path: linux.DefaultSystemctl, Arguments: []string{"--user", "daemon-reload"}},
      {Path: linux.DefaultSystemctl, Arguments: []string{"--user", "enable", linux.UnitName}},
      {Path: linux.DefaultSystemctl, Arguments: []string{"--user", "restart", linux.UnitName}},
  }
  ```

  Add `Registered` tests for absent, present, and permission-error unit paths. Add Linux-only command factory tests proving: systemctl present returns a controller; both systemctl and unit absent return nil/no error; unit present with systemctl absent returns an error; an unreadable unit status returns an error.

- [ ] **Step 2: Run the Linux package test and verify RED**

  Run on Linux: `go test ./internal/platform/linux ./cmd/sshc -run 'Test(Enable|Registered|LinuxService)' -count=1`

  On macOS, compile the Linux packages instead: `GOOS=linux GOARCH=amd64 go test -c ./internal/platform/linux -o /tmp/sshc-linux-loginitem.test && GOOS=linux GOARCH=amd64 go test -c ./cmd/sshc -o /tmp/sshc-linux-cmd.test`

  Expected: native Linux test failure from the old two-command sequence or, on macOS, the command package compile failure until the factory exists. The runnable behavior is subsequently exercised by CI's Linux job.

- [ ] **Step 3: Implement registration status and restart**

  Implement `Registered` using `os.Stat`: present is `(true, nil)`, `os.ErrNotExist` is `(false, nil)`, and every other error is returned. Keep `Enabled()` compatible by ignoring the error for Web status. Change Linux `Enable` to run `daemon-reload`, `enable sshc.service`, and `restart sshc.service` in that order.

  On darwin, construct `macos.LoginItem` with `process.NewOutputRunner()`. On linux, use an injected stat seam to decide whether `/usr/bin/systemctl` is available; if it is absent, call `Registered` and reject a present/unknown unit rather than silently treating it as disabled.

- [ ] **Step 4: Verify platform tests GREEN**

  Run: `go test ./internal/platform/macos ./cmd/sshc -count=1`

  Run: `GOOS=linux GOARCH=amd64 go test -c ./internal/platform/linux -o /tmp/sshc-linux-loginitem.test && GOOS=linux GOARCH=amd64 go test -c ./cmd/sshc -o /tmp/sshc-linux-cmd.test`

  Expected: native tests pass and both Linux test binaries compile.

- [ ] **Step 5: Commit Task 2**

  ```bash
  git add cmd/sshc/service_darwin.go cmd/sshc/service_linux.go cmd/sshc/service_linux_test.go internal/platform/linux/loginitem.go internal/platform/linux/loginitem_test.go
  git commit -m "fix: restart rebound login services"
  ```

### Task 3: Atomic Make installation and guarded uninstall

**Files:**
- Create: `internal/acceptance/install_test.go`
- Modify: `Makefile`
- Modify: `README.md`

**Interfaces:**
- Produces Make target: `install-binary`, consuming `INSTALL_SOURCE` and `INSTALL_DIR`.
- Produces Make target: `uninstall-binary`, consuming `MAINTENANCE_BINARY` and `INSTALL_DIR`.
- `install` calls `install-binary` only after `build`; `uninstall` calls `uninstall-binary` only after `build`.

- [ ] **Step 1: Write failing Make behavior tests**

  In `internal/acceptance/install_test.go`, create executable fixture scripts in `t.TempDir()` and execute Make targets with `exec.Command`. Test observable results:

  ```go
  func TestInstallBinaryAtomicallyReplacesTheCLIAndRefreshesTheService(t *testing.T) {
      // Existing destination contains "old"; fixture source logs "$*" and succeeds.
      // Run: make --no-print-directory install-binary INSTALL_SOURCE=<fixture> INSTALL_DIR=<temp bin>
      // Assert destination equals fixture bytes, mode includes 0111, and log is exactly "service refresh\n".
  }

  func TestInstallBinaryKeepsTheNewCLIButReportsARefreshFailure(t *testing.T) {
      // Fixture exits nonzero for service refresh.
      // Assert make fails, destination is the new fixture, and stderr explains that CLI succeeded but refresh failed.
  }

  func TestInstallBinaryKeepsTheOldCLIWhenStagingFails(t *testing.T) {
      // INSTALL_SOURCE is missing.
      // Assert make fails and destination bytes remain exactly "old".
  }

  func TestUninstallBinaryKeepsTheCLIWhenServiceDisableFails(t *testing.T) {
      // MAINTENANCE_BINARY exits nonzero.
      // Assert make fails and installed destination remains.
  }
  ```

  Add the success counterpart asserting `service disable` is logged before the installed binary disappears. Fixture contents and expected values must be literals independent of Makefile construction.

- [ ] **Step 2: Run acceptance tests and verify RED**

  Run: `go test ./internal/acceptance -run 'Test(InstallBinary|UninstallBinary)' -count=1`

  Expected: FAIL because `install-binary` and `uninstall-binary` do not exist.

- [ ] **Step 3: Implement atomic install and guarded uninstall**

  Add defaults:

  ```make
  INSTALL_SOURCE ?= bin/sshc
  MAINTENANCE_BINARY ?= bin/sshc
  ```

  `install-binary` must create `INSTALL_DIR`, reject a directory at the destination, use `mktemp` to exclusively create a stage file in the destination directory, copy with mode `0755`, install via `mv -f`, clear the cleanup trap, then run the installed `sshc service refresh`. A refresh failure leaves the new CLI installed but exits nonzero with an explicit partial-success message.

  `uninstall-binary` must first run `$(MAINTENANCE_BINARY) service disable`, then remove `$(INSTALL_DIR)/sshc`. `install: build` and `uninstall: build` invoke these primitives with the freshly built `bin/sshc`.

  Update README to say that install rebinds and restarts only an enabled service, the restart locks the vault, disabled startup remains disabled, and uninstall disables the service before removal. Correct the existing UI location from 「秘密」 to 「設定」 and document `sshc service refresh|disable` as maintenance commands.

- [ ] **Step 4: Run acceptance tests and isolated install smoke GREEN**

  Run: `go test ./internal/acceptance -run 'Test(InstallBinary|UninstallBinary)' -count=1`

  Then build with the normal toolchain location and run only the lifecycle steps with a temporary HOME, so neither Go's toolchain lookup nor the live LaunchAgent is redirected:

  ```bash
  smoke_home=$(mktemp -d)
  make build
  HOME="$smoke_home" make install-binary INSTALL_SOURCE="$PWD/bin/sshc" INSTALL_DIR="$smoke_home/.local/bin"
  HOME="$smoke_home" "$smoke_home/.local/bin/sshc" service refresh
  HOME="$smoke_home" make uninstall-binary MAINTENANCE_BINARY="$PWD/bin/sshc" INSTALL_DIR="$smoke_home/.local/bin"
  test ! -e "$smoke_home/.local/bin/sshc"
  ```

  Expected: install and explicit refresh report no enabled service; uninstall succeeds; no plist is created in the temporary HOME.

- [ ] **Step 5: Commit Task 3**

  ```bash
  git add Makefile README.md internal/acceptance/install_test.go
  git commit -m "build: install sshc with its login service"
  ```

### Task 4: Full verification and publication

**Files:**
- Verify all changed files and generated artifacts.

**Interfaces:**
- Consumes: all behavior from Tasks 1–3.
- Produces: a clean, pushed `main` whose origin commit matches local HEAD.

- [ ] **Step 1: Run formatting and generated-file checks**

  Run: `gofmt -w cmd/sshc/service*.go cmd/sshc/connect*.go internal/platform/linux/loginitem*.go internal/acceptance/install_test.go`

  Run: `make verify-generated && git diff --check`

  Expected: generated models unchanged and no whitespace errors.

- [ ] **Step 2: Run normal and race test suites**

  Run: `go test ./... && go test -race ./... && npm test --prefix web && npm run typecheck --prefix web`

  Expected: all Go and Web tests pass with no new warning.

- [ ] **Step 3: Run production build and browser E2E**

  Run: `make build && make e2e`

  Expected: production build succeeds without the prior chunk-size warning; Playwright passes with only declared platform skips.

- [ ] **Step 4: Run Docker-backed integrations and clean them**

  Run: `make integration-up && make integration`

  Always finish with: `make integration-down`

  Expected: real S3 conditional operations and real OpenSSH askpass authentication pass; no `sshc-s3` or `sshc-sshd` container remains.

- [ ] **Step 5: Audit install invariants without changing the live service**

  Verify `git status --short`, inspect the diff from `18c7a74`, confirm package lockfiles are unchanged, and repeat the temporary-HOME install smoke. Do not run `make install` against the user's actual HOME during this task.

- [ ] **Step 6: Commit any verification-only generated changes, then push**

  ```bash
  git status --short
  git push origin main
  git rev-parse HEAD
  git rev-parse origin/main
  ```

  Expected: worktree clean, push succeeds, and local/origin commit IDs match.
