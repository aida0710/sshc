# Native CI, Release, and Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** macOS、Linux、Windows の native test・package smoke・release artifact を必須 gate にし、実証済みの OS/architecture だけを README と release で support として公開する。

**Architecture:** source-level checks と native platform checks を分離し、Go/desktop lifecycle は OS matrix、process ownership と package smoke は native artifact を使う。release は各 OS が自分の package を build/smoke/upload し、最後の publish job が全 artifact と checksum を集約する。documentation は workflow の証拠から supported/verified/unverified を区別する。

**Tech Stack:** GitHub Actions / Go 1.26 / Node 22 / Electron 43 / Playwright / electron-builder / Bash / PowerShell

**Spec:** `docs/superpowers/specs/2026-08-15-explicit-desktop-headless-windows-design.md`

## Global Constraints

- common owner/Vault plan と Windows platform/NSIS plan の production interface を変更せず gate 化する。
- cross-build 成功だけを native support evidence にしない。
- artifact を作った OS 上で smoke test してから publish job へ渡す。
- publish は required macOS/Linux/Windows job の一つでも失敗・skip なら実行しない。
- secret、handoff body、master password、clipboard contents を CI log/artifact に含めない。
- generated `internal/ui/dist`、OpenAPI Go/TypeScript output、package lockfile の一致を維持する。
- Android は unsupported と明記し、untested とだけ書かない。
- macOS unsigned と Windows unsigned/SmartScreen boundary を明記し、署名済みと誤認させない。
- GitHub Action は既存 repository の pinned commit policy を維持する。
- runner label は [GitHub hosted-runner reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners) と [`actions/runner-images` の現行表](https://github.com/actions/runner-images/blob/main/README.md)を根拠に固定し、deprecated `macos-14` ではなく `macos-15` / `macos-15-intel` を使う。
- CI runner へ必要な tool を入れる場合も workflow 内に限定し、開発者のローカル環境へ package install を要求しない。

## Parallel Execution Map

実装は同一 file の同時編集を避け、次の wave で並行化する。

```text
Wave 0 (直列)
  Common Task 1-2: invocation + handoff/protocol interface

Wave 1 (並行)
  Track A: Common Task 3-4  Vault HTTP/CLI
  Track B: Common Task 5    engine runner/ownership channel
  Track C: Windows Task 1-2 filesystem/DACL foundation

Wave 2 (並行)
  Track A: Common Task 6-7  activation/connection state machine
  Track B: Common Task 8-9  Electron lifecycle/stable CLI
  Track C: Windows Task 3-5 lock/toolchain/agent
  Track D: This plan Task 1-2 Make targets/native source CI

Wave 3 (並行)
  Track A: Common Task 10 process integration
  Track B: Windows Task 6-8 ConPTY/launcher/NSIS
  Track C: This plan Task 3-4 native process/package smoke

Wave 4 (直列統合)
  Windows Task 9 + This plan Task 5-7 release/docs/final gates
```

各 track は専用 commit を作る。shared files (`cmd/sshc/main.go`, `internal/app/run.go`, `desktop/main.js`, `Makefile`, workflow files, `README.md`) の変更は担当を一人に固定し、他 track は新規 test/helper file を先に作る。wave 終了ごとに full test を通してから次へ進む。

---

### Task 1: Makefile を native build/package entry point に分ける

**Files:**
- Modify: `Makefile`
- Create: `scripts/verify-artifact-name.sh`
- Create: `scripts/verify-artifact-name.ps1`
- Create: `internal/buildcontract/makefile_test.go`

**Interfaces:**

```text
make build-cli GOOS=$TARGET_GOOS GOARCH=$TARGET_GOARCH OUTPUT=$TARGET_OUTPUT
make desktop-bundle-mac
make desktop-bundle-linux
make desktop-bundle-windows
make release-cli-current
```

- [ ] **Step 1: target/architecture contract の失敗テストを書く**

`internal/buildcontract/makefile_test.go` は Makefile text を parse し、darwin/linux/windows の amd64/arm64 target、Windows `.exe` suffix、desktop resource directory、OS-specific desktop script がすべて列挙されることを検査する。shell 実装そのものを Go test で再実装しない。

- [ ] **Step 2: current four-target Makefile で落ちることを確認する**

Run: `go test ./internal/buildcontract`
Expected: FAIL（Windows target と native desktop target が無い）

- [ ] **Step 3: generic CLI build primitive を追加する**

`build-cli` は `GOOS`、`GOARCH`、`OUTPUT` の空値を拒否し、`CGO_ENABLED` は caller が明示する。version ldflag と `-trimpath` は現行と同じ一箇所で組み立てる。output path を glob や HOME から決めない。

- [ ] **Step 4: desktop bundle targets を OS 別にする**

- macOS runner: darwin arm64/amd64 CLI resource + `npm run dist:mac`。
- Linux runner: linux amd64/arm64 CLI resource + `npm run dist:linux`。
- Windows runner: windows amd64/arm64 CLI `.exe` resource + `npm run dist:win`。

一つの runner が別 OS の Electron package を作る旧 `desktop-dist` は削除する。development `desktop-run` は host build を使い続ける。

- [ ] **Step 5: current-OS standalone target を追加する**

release job は自 OS の二 architecture を build して artifact directory に置く。darwin cgo requirement は macOS runner 内で満たし、Linux/Windows は `CGO_ENABLED=0` とする。

- [ ] **Step 6: target tests と host regression を通す**

Run: `go test ./internal/buildcontract`
Expected: PASS

Run: `make build`
Expected: PASS

Run: `make test`
Expected: PASS

- [ ] **Step 7: commit**

```bash
git add Makefile scripts/verify-artifact-name.sh scripts/verify-artifact-name.ps1 internal/buildcontract/makefile_test.go
git commit -m "build: split native cli and desktop targets"
```

---

### Task 2: Go と desktop lifecycle を三 OS native CI matrix にする

**Files:**
- Modify: `.github/workflows/ci.yml`
- Create: `internal/buildcontract/workflow_test.go`
- Create: `scripts/ci/check-gofmt.sh`
- Create: `scripts/ci/check-gofmt.ps1`
- Modify: `desktop/package.json`
- Modify: `desktop/package-lock.json`

**Interfaces:**

```yaml
strategy:
  fail-fast: false
  matrix:
    include:
      - os: ubuntu-24.04
        name: Linux
        race: true
      - os: macos-15
        name: macOS
        race: true
      - os: windows-2025
        name: Windows
        race: true
```

- [ ] **Step 1: workflow structural test を失敗する形で追加する**

`internal/buildcontract/workflow_test.go` は既存 direct dependency `gopkg.in/yaml.v3` で `.github/workflows/ci.yml` を decode し、jobs matrix、OS list、required commands を構造として検査する。文字列 grep や新 dependency は使わない。

検査項目:

- Go native job に `ubuntu-24.04`、`macos-15`、`windows-2025` がある。
- 各 OS で vet/build/test。
- Linux/macOS で race mandatory。
- Windows race は実行を試し、toolchain unsupported 以外を failure にする。
- desktop lifecycle Node tests が三 OS で走る。
- source/generated/web checks は重複せず一回のまま。

- [ ] **Step 2: current CI に Windows がなく落ちることを確認する**

Run: repository workflow contract test command established in Step 1
Expected: FAIL

- [ ] **Step 3: native Go matrix を実装する**

Windows shell を PowerShell に固定し、Unix shell fragment を共有しない。gofmt output check は OS-specific script を使う。`go env` と `go version` は evidence として出してよいが、環境全体を dump しない。

- [ ] **Step 4: Windows race availability を事実として判定する**

`go test -race ./...` を実行し、exit output が Go toolchain の明示的 unsupported message の場合だけ neutral `Race unavailable on this runner/toolchain` と記録する。test failure、compile failure、data race は必ず job failure にする。Linux/macOS は例外なく mandatory。

- [ ] **Step 5: desktop lifecycle matrix を実装する**

Node setup/cache と `npm ci --prefix desktop`、`npm test --prefix desktop` を各 OS で実行する。これは Electron window smoke ではなく pure Node lifecycle contract であることを job name に表す。

- [ ] **Step 6: workflow contract、actionlint 相当、local tests を通す**

Run: repository workflow contract test
Expected: PASS

Run: `npm test --prefix desktop`
Expected: PASS

Run: `go test ./...`
Expected: PASS

- [ ] **Step 7: commit**

```bash
git add .github/workflows/ci.yml internal/buildcontract/workflow_test.go scripts/ci desktop/package.json desktop/package-lock.json
git commit -m "ci: test go and desktop lifecycle on three systems"
```

---

### Task 3: native process ownership/Vault integration jobs を追加する

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `integration/engine_ownership_test.go`
- Modify: `integration/vault_cli_test.go`
- Modify: `integration/desktop_wait_test.go`
- Create: `scripts/ci/run-process-integration.sh`
- Create: `scripts/ci/run-process-integration.ps1`

**Interfaces:**

```text
go test ./integration -tags=processintegration -count=1 -v
```

- [ ] **Step 1: OS-native process integration job contract test を追加する**

CI workflow に Ubuntu/macOS/Windows matrix、real binary build、isolated HOME/state、process integration tag が揃わないと落ちる test を書く。

- [ ] **Step 2: process tests を portable command harness に揃える**

Unix signal/process-group assertion と Windows Ctrl/Job assertion を build-tagged helper に分ける。test timeout は各 process に持たせ、suite 全体の arbitrary sleep に頼らない。cleanup は test が起動した exact PID/process handle と temp root だけを対象にする。

- [ ] **Step 3: required cases を OS matrix で固定する**

全 OS 共通:

- bare CLI が lock を持たない。
- headless locked/missing announcement は secret を含まない。
- owner exclusion と stale handoff replacement。
- desktop ownership pipe EOF。
- Vault create/unlock/lock/change、non-TTY refusal、confirmation mismatch。
- desktop locked wait → external unlock → same CLI continuation。
- cancellation と owner/protocol change。

OS-specific:

- Unix SIGINT/SIGTERM。
- Windows Ctrl-C/Ctrl-Break と `LockFileEx` release。

- [ ] **Step 4: secret leak scanner を artifact-safe にする**

canary 自体を log せず、その SHA-256 を test memory 内で比較する。argv、child environment、stdout/stderr、state files、temp files を scan し、一致時は path と channel だけを報告する。

- [ ] **Step 5: native matrix job を追加する**

test binary と failure logs には retention 7日を設定する。ただし HOME/state artifact は upload しない。Windows job は PowerShell runner script を使う。

- [ ] **Step 6: current OS process suite と workflow contract を通す**

Run: `go test ./integration -tags=processintegration -count=1 -v`
Expected: PASS on current OS

Run: repository workflow contract test
Expected: PASS

- [ ] **Step 7: commit**

```bash
git add .github/workflows/ci.yml integration scripts/ci/run-process-integration.sh scripts/ci/run-process-integration.ps1
git commit -m "ci: verify engine ownership with native processes"
```

---

### Task 4: packaged Electron smoke を各 OS で実行する

**Files:**
- Modify: `.github/workflows/ci.yml`
- Create: `scripts/macos/package-smoke.sh`
- Create: `scripts/linux/package-smoke.sh`
- Modify: `scripts/windows/package-smoke.ps1`
- Create: `web/e2e/packaged-desktop.spec.ts`
- Modify: `web/e2e/support/environment.ts`
- Modify: `web/playwright.config.ts`
- Modify: `desktop/package.json`

**Interfaces:**

```text
SSHC_PACKAGED_APP=$PACKAGED_APP_PATH npm run e2e:packaged --prefix web
```

- [ ] **Step 1: packaged-vs-development guard の失敗テストを書く**

packaged suite は `SSHC_PACKAGED_APP` が repository の Electron executable や `node_modules/electron` を指す場合に拒否する。app binary/resource CLI の absolute path と package metadata version が一致することを開始前に検査する。

- [ ] **Step 2: shared packaged lifecycle/clipboard cases を追加する**

- first launch で engine child は一台。
- window close 後も app/engine が残る。
- bare CLI/second launch で restore/focus。
- explicit Quit で engine が終了。
- copy-on-select enabled/disabled。
- right-click-paste enabled/disabled。
- clipboard refusal に内容を含まない error。

clipboard assertion は system clipboard canary を test 内で比較し、report attachment に値を載せない。

- [ ] **Step 3: macOS DMG smoke script を実装する**

DMG を attach して app を temp Applications-like directory へ copy し、LaunchServices `/usr/bin/open -b` activation、background close、Cmd+Q path、managed CLI copy を検査する。終了時は test mount と test app copy だけを detach/remove する。

- [ ] **Step 4: Linux AppImage smoke script を実装する**

Xvfb 上で AppImage を起動し、original `APPIMAGE` descriptor、stable managed CLI、moved-image error、single instance、tray present/absent background behavior を検査する。Wayland runner が提供される workflow では同じ suite を native Wayland で走らせ、無い場合は X11 result と Wayland unverified を区別する。

- [ ] **Step 5: Windows NSIS smoke と shared Playwright を接続する**

Windows plan の silent install script から installed Electron path を packaged suite に渡す。HKCU/PATH/ConPTY/ACL assertion と window/clipboard suite の両方が通って初めて Windows package smoke success とする。

- [ ] **Step 6: pull-request CI の package smoke matrix を追加する**

各 architecture job が native package を build して即 smoke する。artifact は failure diagnosis と release dry-run 用に7日保持する。matrix は Linux x64=`ubuntu-24.04`、Linux arm64=`ubuntu-24.04-arm`、macOS x64=`macos-15-intel`、macOS arm64=`macos-15`、Windows x64=`windows-2025`、Windows arm64=`windows-11-arm` を使う。public-preview runner の unavailable/infra failure を product success と扱わず、その architecture は unverified のままにする。

- [ ] **Step 7: current available OS の packaged smoke を通す**

macOS:

```bash
make desktop-bundle-mac
scripts/macos/package-smoke.sh "$SSHC_DMG_PATH"
```

Linux:

```bash
make desktop-bundle-linux
xvfb-run --auto-servernum scripts/linux/package-smoke.sh "$SSHC_APPIMAGE_PATH"
```

Windows:

```powershell
make desktop-bundle-windows
pwsh -File scripts/windows/package-smoke.ps1 -Installer $env:SSHC_NSIS_X64 -Architecture x64 -WorkRoot $env:TEMP\sshc-package-smoke
```

Expected: target OS の command が PASS

- [ ] **Step 8: commit**

```bash
git add .github/workflows/ci.yml scripts/macos scripts/linux scripts/windows/package-smoke.ps1 web/e2e/packaged-desktop.spec.ts web/e2e/support/environment.ts web/playwright.config.ts desktop/package.json
git commit -m "ci: smoke native desktop packages"
```

---

### Task 5: release workflow を native build/smoke/publish に分割する

**Files:**
- Modify: `.github/workflows/release.yml`
- Create: `scripts/release/check-artifacts.sh`
- Create: `scripts/release/check-artifacts.ps1`
- Create: `scripts/release/check-artifacts.test.js`
- Modify: `Makefile`

**Interfaces:**

```text
jobs:
  macos-artifacts
  linux-artifacts
  windows-artifacts
  publish (needs all three)
```

- [ ] **Step 1: release DAG と artifact completeness の失敗テストを書く**

test fixture manifest に required artifact patterns を定義する。

```json
{
  "required": [
    "sshc-darwin-arm64",
    "sshc-darwin-amd64",
    "sshc-linux-arm64",
    "sshc-linux-amd64",
    "sshc-windows-arm64.exe",
    "sshc-windows-amd64.exe",
    ".dmg",
    ".AppImage",
    ".exe"
  ]
}
```

duplicate name、missing OS、zero-byte file、unexpected architecture、version mismatch で checker が落ちることを検査する。

- [ ] **Step 2: single macOS release job のため contract test が落ちることを確認する**

Run: `node --test scripts/release/check-artifacts.test.js`
Expected: FAIL

- [ ] **Step 3: three native artifact jobs を実装する**

`verify-source` job が checkout/setup、Go/Web/Desktop の source gates と generated-file check を一度だけ行う。各 native job は `needs: verify-source` とし、current-OS binaries、desktop package、package smoke、artifact upload の順に実行する。package smoke は必ず artifact job 内で行う。

- [ ] **Step 4: artifact provenance を metadata に含める**

artifact bundle に tag、commit SHA、runner OS、target architecture、smoke result を含む機械可読 manifest を置く。environment dump や secret は含めない。publish checker は全 manifest の tag/SHA 一致を検査する。

- [ ] **Step 5: publish job を aggregate-only にする**

`needs: [verify-source, macos-artifacts, linux-artifacts, windows-artifacts]` とし、全 artifact を download、completeness check、SHA-256 checksum 作成、GitHub release 作成だけを行う。publish job 自身は package を再buildしない。

- [ ] **Step 6: unverified architecture policy を実装する**

native/real-machine smoke evidence がない architecture は release manifest に `verified: false` を持たせ、既定 publish set から除外する。build-only artifact を出す場合は filename/release note に `unverified` を含める。x64 success から arm64 success を推論しない。

- [ ] **Step 7: workflow/checker tests を通す**

Run: `node --test scripts/release/check-artifacts.test.js`
Expected: PASS

Run: repository workflow contract test
Expected: PASS

Run: checker against locally assembled fixture artifacts
Expected: PASS

- [ ] **Step 8: commit**

```bash
git add .github/workflows/release.yml scripts/release Makefile
git commit -m "release: publish only native-smoked artifacts"
```

---

### Task 6: README/help/manual matrix を実装と一致させる

**Files:**
- Modify: `README.md`
- Modify: `cmd/sshc/invocation.go`
- Modify: `cmd/sshc/invocation_test.go`
- Create: `docs/manual-test-matrix.md`
- Create: `docs/headless-examples.md`

**Interfaces:**

README support table columns:

```text
OS | Architecture | CLI/headless | Desktop package | Native smoke | Notes
```

- [ ] **Step 1: documentation contract の失敗テストを書く**

test は README/help が次の語義を持つことを検査する。

- `sshc engine` が「何をするか」だけでなく Electron ownership のために存在する理由。
- bare `sshc` は desktop activation で engine ではない。
- headless は foreground で別 terminal の Vault CLI を案内。
- vault password は TTY-only、8時間 inactivity。
- Linux autostart に Dock を使わず GNOME/KDE/Xfce 等の desktop startup settings を案内。
- Windows per-user NSIS/PATH/Startup Apps/SmartScreen。
- Android unsupported。

旧 `--own-engine` と bare engine の利用案内が残ると落ちる。

- [ ] **Step 2: README/help の現状差分で落ちることを確認する**

Run: documentation contract test command
Expected: FAIL

- [ ] **Step 3: command/lifecycle/vault documentation を書く**

desktop graphical flow と no-display headless flow を別 section にし、Linux GUI/非GUIを混同しない。`sshc <alias>` locked desktop wait と headless promptless failure を例示する。passwordless ordinary connection には `ssh` を使えることを記載する。

- [ ] **Step 4: OS startup guidance を具体化する**

- macOS: System Settings Login Items または Dock options。
- Linux: desktop environment の Autostart/Startup Applications から AppImage absolute path を登録し、sshc は unit を生成しない。
- Windows: Settings Apps > Startup で installer app を管理し、sshc は Windows service を入れない。

distribution/desktop version により文言が変わる箇所は OS vendor UI 名に依存しすぎない書き方にする。

- [ ] **Step 5: headless supervisor examples を安全に書く**

systemd、tmux、Docker、PowerShell/Task Scheduler wrapper は foreground `sshc headless` を所有する例にする。password を unit、environment、command line に書かず、起動後に別 TTY から `sshc vault unlock` を実行する。

- [ ] **Step 6: verified support matrix を CI evidence から埋める**

workflow run URL/commit と実行日を manual matrix に記録し、pass していない architecture は unverified/unsupported のいずれかを明記する。artifact build 成功だけで native smoke 欄を yes にしない。

- [ ] **Step 7: documentation tests と help snapshot を通す**

Run: `go test ./cmd/sshc -run 'TestUsage|TestDocumentation'`
Expected: PASS

Run: documentation contract test
Expected: PASS

Run: `rg -n -- '--own-engine|run the engine here|Linux.*Dock|bare.*engine' README.md docs cmd/sshc`
Expected: historical superseded spec 以外に contradictory instruction が無い

- [ ] **Step 8: commit**

```bash
git add README.md cmd/sshc/invocation.go cmd/sshc/invocation_test.go docs/manual-test-matrix.md docs/headless-examples.md
git commit -m "docs: explain desktop and headless ownership on every os"
```

---

### Task 7: final native gate と release dry run を実行する

**Files:**
- Modify: `.github/workflows/ci.yml` only for evidence-backed fixes
- Modify: `.github/workflows/release.yml` only for evidence-backed fixes
- Modify: `docs/manual-test-matrix.md`
- Modify: `README.md` only to correct verified boundaries

**Interfaces:**

新しい production interface は作らない。このタスクの出力は required workflow result、artifact manifest、`docs/manual-test-matrix.md` の evidence row である。

- [ ] **Step 1: repository-wide local/static gate を通す**

Run:

```bash
gofmt -w cmd internal integration
go vet ./...
go test ./...
go test -race ./...
go build ./...
npm test --prefix web
npm run typecheck --prefix web
npm test --prefix desktop
make verify-generated
make build
git diff --exit-code -- internal/ui/dist
```

Expected: PASS。`internal/ui/dist` に source build 由来の必要差分がある場合は regenerateして commitし、その後 clean。

- [ ] **Step 2: real sshd integration を通す**

Run on Linux CI:

```bash
make integration-up
make integration
make integration-down
```

Expected: password、key passphrase、ProxyJump chain、remaining interactive authentication の controlled sshd tests が PASS。cleanup は `if: always()` でも走る。

- [ ] **Step 3: three-OS native CI を main candidate commit で通す**

required jobs:

- Go Linux/macOS/Windows
- desktop lifecycle Linux/macOS/Windows
- native process integration Linux/macOS/Windows
- web/generated/integration/security
- packaged desktop smoke Linux/macOS/Windows

Expected: all required PASS。skipped required job を green と扱わない。

- [ ] **Step 4: release workflow dry run を non-publishing mode で通す**

workflow_dispatch または local artifact fixture で macOS/Linux/Windows artifact job と aggregate checker を実行し、publish step だけを無効にする。全 artifact の SHA/tag/version/architecture/smoke manifest 一致を確認する。

- [ ] **Step 5: manual real-machine matrix を埋める**

native runner で自動化できない tray absence、Wayland、LaunchServices、Windows Startup Apps、SmartScreen first run、arm64 architecture を real machine で実行する。未実行は空欄でなく `not verified` と理由を記録する。

- [ ] **Step 6: security regression を review する**

Run:

```bash
rg -n 'ReadPassword|passphrase|password|Secret|#bootstrap=' cmd internal desktop scripts .github
rg -n 'exec.Command|spawn\(|CreateProcess|PATH|ComSpec|APPIMAGE' cmd internal desktop scripts
```

Expected: password の argv/env/log path、bootstrap URL の headless log、shell-based launcher、Windows PATH search、AppImage mount symlink が無い。fixture values は明確に test-only。

- [ ] **Step 7: support claim を最終証拠に合わせる**

CI/run/manual matrix の pass だけを README `Supported` に反映する。不合格または未実行を隠さず、release artifact を withheld または unverified label にする。

- [ ] **Step 8: final commit**

```bash
git add .github/workflows/ci.yml .github/workflows/release.yml docs/manual-test-matrix.md README.md internal/ui/dist
git commit -m "chore: record native release verification"
```

---

## Completion Gate

- [ ] Go vet/build/test が Linux、macOS、Windows native で通る。
- [ ] race が Linux/macOS で通り、Windows は pass または toolchain unsupported evidence がある。
- [ ] engine ownership/Vault process integration が三 OS で通る。
- [ ] DMG、AppImage、NSIS が各 native OS 上で package smoke を通る。
- [ ] packaged Electron の background/quit/clipboard behavior が三 OS で通る。
- [ ] release publish job は三 native artifact job 全成功を必須にする。
- [ ] checksums と artifact manifests が tag/commit/version/architecture を一致させる。
- [ ] README/help が実装済み command と GUI/headless distinction に一致する。
- [ ] Linux autostart guidance と Windows Startup Apps guidance がある。
- [ ] Android が unsupported と明記される。
- [ ] OS/architecture support claim が native/real-machine evidence を超えていない。
