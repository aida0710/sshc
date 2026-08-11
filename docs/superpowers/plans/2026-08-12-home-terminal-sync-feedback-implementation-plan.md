# Home Actions, Terminal Preference, and Sync Feedback Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Home connection actions explicit, move the global terminal preference to Settings, and report exact S3 snapshot and transfer results.

**Architecture:** Keep the three UI concerns separate: a focused Home action menu, a narrow metadata mutation for the global terminal preference, and typed remote-sync operation summaries measured by the server. Extend the existing OpenAPI and web client contracts without exposing snapshot contents or secrets, and persist only the last successful push/apply summary in the backward-compatible sync state.

**Tech Stack:** Go, Echo, React, TypeScript, Vitest/Testing Library, Playwright, OpenAPI, Vite, Docker-backed S3/OpenSSH integration

## Global Constraints

- Do not add or install packages.
- Keep `sshc` foreground-only; this work does not add a background daemon.
- Home must not start SSH until the explicit `接続` menu item is selected.
- Terminal preference updates may change only `metadata.terminal` and `metadata.customTerminal` from freshly loaded metadata.
- Custom terminal execution remains an application path plus validated argv and never becomes a shell command.
- Byte counts use decimal units in the UI: 1 MB is 1,000,000 bytes.
- Preview must not persist a last operation; only successful push and apply do.
- A failed operation must not erase or replace the previous successful operation.
- Do not return or log file contents, private-key material, passphrases, or S3 credentials.
- Refresh the embedded production assets in `internal/ui/dist` and verify Docker integration before pushing `main`.

---

### Task 1: Home Connection Action Menu

**Files:**
- Create: `web/src/overview/ConnectionActions.tsx`
- Create: `web/src/overview/ConnectionActions.test.tsx`
- Modify: `web/src/overview/OverviewPanel.tsx`
- Modify: `web/src/overview/OverviewPanel.test.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/i18n/catalog.ts`

**Interfaces:**
- Consumes: `connectionLocation({ view: "servers" }, { path, alias, panel: "Basic", advanced: "Jump" })`, `IntegrationsApi.terminalLaunch(alias)`, and App's location navigator.
- Produces: `ConnectionActions` with alias/path/busy and settings/connect callbacks.

- [ ] **Step 1: Write failing interaction tests**

  Assert that opening `<alias> の操作` does not call `terminalLaunch`; `接続設定を開く` emits the exact Basic-panel location including path and alias; only `接続` launches; Escape and outside pointer close the menu; and a pending launch disables duplicate connection actions without disabling settings navigation.

- [ ] **Step 2: Run focused tests and confirm RED**

  Run: `cd web && npm test -- --run src/overview/ConnectionActions.test.tsx src/overview/OverviewPanel.test.tsx`

  Expected: FAIL because the menu and navigation prop do not exist.

- [ ] **Step 3: Implement the accessible menu and wire Home**

  Keep open state inside `ConnectionActions`, close on choice, Escape, and outside pointerdown, and give the trigger an alias-specific accessible name. Replace the card's direct button, pass App's location navigator through, and generate destinations only with `connectionLocation`.

- [ ] **Step 4: Verify and commit**

  Run: `cd web && npm test -- --run src/overview/ConnectionActions.test.tsx src/overview/OverviewPanel.test.tsx && npm run typecheck`

  Expected: PASS. Then run: `git add web/src && git commit -m "feat: clarify home connection actions"`

### Task 2: Narrow Terminal Preference Service and API

**Files:**
- Modify: `internal/application/service.go`
- Modify: `internal/application/service_test.go`
- Modify: `internal/httpserver/diagnostics.go`
- Modify: `internal/httpserver/diagnostics_test.go`
- Modify: `internal/httpserver/server.go`
- Modify: `api/openapi.yaml`

**Interfaces:**
- Produces: `func (s *Service) SetPreferredTerminal(choice platform.TerminalChoice) (bool, error)`.
- Produces: `PUT /api/v1/terminal/preference` accepting selected and optional custom-terminal fields, then returning refreshed terminal options.

- [ ] **Step 1: Write failing service tests**

  Cover standard/custom updates, preservation of unrelated host/group metadata loaded immediately before commit, invalid choices, and exact equality returning `changed=false` without a journal entry.

- [ ] **Step 2: Run service tests and confirm RED**

  Run: `go test ./internal/application -run 'TestServiceSetPreferredTerminal' -count=1`

  Expected: FAIL because the method is undefined.

- [ ] **Step 3: Implement the metadata-only mutation**

  Under `saveMutex`, validate the choice, load current metadata, compare terminal ID/application/argv, and no-op on equality. Otherwise clone current metadata, replace only terminal fields, and commit one `storage.Request{Operation: "terminal.preference"}` through the existing metadata change and preconditions.

- [ ] **Step 4: Write failing HTTP tests**

  Cover valid standard/custom requests, malformed JSON, invalid/unavailable choices, unavailable setter, commit failure, no-op, and a response containing saved custom details.

- [ ] **Step 5: Add route, injection, and OpenAPI schema**

  Supply the setter from `options.Config.SetPreferredTerminal`; map invalid request to 400, unavailable service to 503, and commit failure to 500; refresh options after success. Define the request/custom-terminal/response fields in OpenAPI.

- [ ] **Step 6: Verify and commit**

  Run: `go test ./internal/application ./internal/httpserver -count=1 && make verify-generated`

  Expected: PASS. Then run: `git add internal/application internal/httpserver api && git commit -m "feat: add terminal preference endpoint"`

### Task 3: Settings Owns the Global Terminal Preference

**Files:**
- Create: `web/src/settings/TerminalPreferenceSection.tsx`
- Create: `web/src/settings/TerminalPreferenceSection.test.tsx`
- Modify: `web/src/settings/SettingsPanel.tsx`
- Modify: `web/src/settings/SettingsPanel.test.tsx`
- Modify: `web/src/connections/ConnectionsPage.tsx`
- Modify: `web/src/connections/ConnectionSummary.tsx`
- Modify: `web/src/api/integrations.ts`
- Modify: `web/src/api/integrations.test.ts`
- Modify: `web/src/i18n/catalog.ts`

**Interfaces:**
- Consumes: terminal options and preference endpoints from Task 2.
- Produces: `IntegrationsApi.setTerminalPreference(request): Promise<TerminalOptions>` and `TerminalPreferenceSection` as the sole editor.

- [ ] **Step 1: Write failing client tests**

  Assert the client sends only preference fields, accepts valid custom-terminal responses, and rejects missing or ill-typed selected, terminal, application, and argument values.

- [ ] **Step 2: Implement and verify the typed client**

  Reuse one strict options validator for GET and PUT, and use the existing authenticated JSON request helper. Run: `cd web && npm test -- --run src/api/integrations.test.ts`. Expected: PASS after the initial RED run.

- [ ] **Step 3: Write failing Settings and Connections tests**

  Cover current/installed/unavailable options, custom application and argv editing, save success, rollback to the saved server value on failure, and absence of editable terminal controls in Connections while unavailable-launcher feedback remains.

- [ ] **Step 4: Implement the Settings editor and remove duplicate controls**

  Load server options on mount, save known choices immediately, and save custom only with a selected application and validated argv. Expose busy state; on error restore/refetch the last server response and show a danger notice. Keep Connections read-only for launcher availability.

- [ ] **Step 5: Verify and commit**

  Run: `cd web && npm test -- --run src/api/integrations.test.ts src/settings src/connections && npm run typecheck`

  Expected: PASS. Then run: `git add web/src && git commit -m "feat: move terminal preference to settings"`

### Task 4: Measure and Persist Remote-Sync Results

**Files:**
- Modify: `internal/remotesync/service.go`
- Modify: `internal/remotesync/service_test.go`
- Modify: `internal/remotesync/snapshot.go`
- Modify: `internal/remotesync/snapshot_test.go`
- Modify: `internal/remotesync/s3_integration_test.go`

**Interfaces:**
- Produces: `SnapshotSummary{CreatedAt, FileCount, SourceBytes, SnapshotBytes}`.
- Produces: `PushResult{Summary, ObjectCount, UploadedBytes, CompletedAt}` and `Push(ctx) (PushResult, error)`.
- Extends: `PullResult` with summary, downloaded bytes, and completion time.
- Extends state with optional last-operation data for successful push/apply.

- [ ] **Step 1: Write failing measurement and compatibility tests**

  Use exact fixture lengths to prove source bytes exclude archive overhead, snapshot bytes equal the sealed body, full push reports two objects/twice the body, pull reports GET body size, preview leaves state unchanged, apply persists its own result, failures preserve the prior success, and legacy JSON still loads.

- [ ] **Step 2: Run tests and confirm RED**

  Run: `go test ./internal/remotesync -count=1`

  Expected: compile/test failures for absent result fields and the changed Push signature.

- [ ] **Step 3: Implement measurements from actual bytes**

  Sum collected content slices for source bytes, use `len(sealed)` for one snapshot, increment upload count/bytes only after successful PUTs, and use `len(object.Body)` for downloads. Take creation time from the manifest and completion time from the service clock.

- [ ] **Step 4: Persist only complete successes**

  Add an optional last-operation JSON field. Write push state only after both PUTs and apply state only after the workspace transaction; never update it for preview, conflicts, remote-moved, or other failures.

- [ ] **Step 5: Update callers/integration assertions, verify, and commit**

  Update all Push callers and S3 tests. Run: `go test ./internal/remotesync -count=1`. Expected: PASS. Then run: `git add internal/remotesync && git commit -m "feat: measure remote sync operations"`

### Task 5: Expose Sync Results Through API and Web Client

**Files:**
- Modify: `internal/httpserver/sync.go`
- Modify: `internal/httpserver/sync_test.go`
- Modify: `api/openapi.yaml`
- Modify: `web/src/api/integrations.ts`
- Modify: `web/src/api/integrations.test.ts`

**Interfaces:**
- Produces: `PushResponse{status, result}`.
- Extends: `SyncStatus.lastOperation` and `PullResponse.summary`, downloaded bytes, and completion time.
- Produces strict TypeScript validators for all non-negative byte/count integers, timestamps, summaries, and operation results.

- [ ] **Step 1: Write failing HTTP tests**

  Assert push returns exact result metrics; pull returns summary/download/change fields; status has optional last success; and remote-moved warns that history may remain although live was not updated.

- [ ] **Step 2: Implement domain-to-API mapping**

  Make one status builder used by GET and push. Copy measured values without recomputation, preserve relative paths, and keep prior successful state independent from current errors.

- [ ] **Step 3: Extend OpenAPI and write failing client validators**

  Define required schemas, then test valid payloads plus negative, fractional, missing, and wrong-type metric fields.

- [ ] **Step 4: Implement client types/validators and verify**

  Change `pushSnapshot` to return `PushResponse` and extend pull/status parsing. Run: `go test ./internal/httpserver -count=1 && make verify-generated && cd web && npm test -- --run src/api/integrations.test.ts && npm run typecheck`. Expected: PASS after initial RED runs.

- [ ] **Step 5: Commit sync contracts**

  Run: `git add internal/httpserver api web/src/api && git commit -m "feat: expose remote sync results"`

### Task 6: Render Structured Sync Result Cards

**Files:**
- Create: `web/src/sync/SyncResultCard.tsx`
- Create: `web/src/sync/SyncResultCard.test.tsx`
- Modify: `web/src/sync/SyncPanel.tsx`
- Modify: `web/src/sync/SyncPanel.test.tsx`
- Modify: `web/src/i18n/catalog.ts`

**Interfaces:**
- Consumes: push, pull, and status result types from Task 5.
- Produces: decimal byte formatting and a card for current push/preview/apply/conflict or persisted prior success.

- [ ] **Step 1: Write failing presentation tests**

  Cover decimal boundaries, localized counts/dates, source/snapshot/two-object transfer, preview download/expanded/change counts, apply wording and second download, conflicts, legacy unknown fields, and `前回の成功` after reload.

- [ ] **Step 2: Implement result formatting and panel state**

  Keep the latest operation result separate from status. A failed request adds a danger notice without clearing prior success. Label preview and apply downloads separately and say `スナップショットを適用`, never `完全復元`.

- [ ] **Step 3: Verify and commit**

  Run: `cd web && npm test -- --run src/sync/SyncResultCard.test.tsx src/sync/SyncPanel.test.tsx && npm test -- --run && npm run typecheck`

  Expected: PASS. Then run: `git add web/src/sync web/src/i18n && git commit -m "feat: show detailed sync results"`

### Task 7: End-to-End, Generated Assets, Docker, and Publish

**Files:**
- Modify: existing files under `web/e2e/` for Home, Settings, and sync journeys.
- Modify: `internal/ui/dist/*` only through the repository production build.

**Interfaces:**
- Verifies all interfaces from Tasks 1-6 in the embedded application.

- [ ] **Step 1: Add/update Playwright journeys and run them**

  Verify canonical Home settings navigation, no launch on menu open, launch only on explicit connect, one global terminal preference shared by connection entry points, and distinct push/preview/apply/prior-success/failure cards. Run: `make e2e`. Expected: all supported scenarios PASS with only documented platform skips.

- [ ] **Step 2: Run Docker-backed integration with cleanup**

  Confirm `sshc-s3` and `sshc-sshd` are absent, then run: `trap 'make integration-down' EXIT; make integration-up; make integration`. Expected: S3/OpenSSH integration PASS and both containers are removed by the trap.

- [ ] **Step 3: Run full verification**

  Run: `make verify-generated && make test && git diff --check`. Expected: generated checks, Go normal/race, Web tests/typecheck/build, and whitespace checks PASS.

- [ ] **Step 4: Refresh and inspect embedded assets**

  Run the repository production build target, verify no stale asset references or large-chunk warning, and confirm no dependency lock changes. Commit with: `git add internal/ui/dist && git commit -m "build: refresh web assets"`.

- [ ] **Step 5: Final review and publish main**

  Inspect status and every commit since `origin/main`, fetch the remote, require a clean fast-forward relationship, push `main`, and compare local/remote hashes. Do not push after a failed check or with unexpected user changes.
