# Explicit Key and Stored Password Exclusion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Treat a directly configured SSH private key and a stored remote-account password as mutually exclusive, while preserving ordinary OpenSSH manual prompts and the independent key-passphrase vault.

**Architecture:** Derive one server-owned policy from the first concrete `Host` block for an alias, not from the effective inherited `IdentityFile`. Apply the same policy at password-save, connection-update, token-issue, terminal-launch, and askpass-redemption boundaries. Extend the existing config/vault transaction so a no-op password removal can still hold the vault writer lock across a config commit, then make the React form render either direct-key controls or stored-account-password controls from the same direct-field rule.

**Tech Stack:** Go, Echo, React, TypeScript, Vitest/Testing Library, Playwright, OpenAPI-generated types, Vite, Docker-backed OpenSSH integration

## Global Constraints

- Do not install or add packages.
- A direct `IdentityFile` with at least one value other than `none` excludes stored account passwords.
- `IdentityFile none`, an inherited `IdentityFile`, and ssh-agent identities do not exclude stored account passwords.
- `CertificateFile`, `IdentityAgent`, and `IdentitiesOnly` alone do not activate this policy.
- Do not add `PasswordAuthentication no`, `PreferredAuthentications`, `BatchMode`, or any other SSH option to normal connection launches.
- Authentication diagnostics remain non-interactive and continue using their existing `BatchMode` behavior.
- An explicit key may still fail into ordinary OpenSSH terminal prompts; sshc only withholds its saved account password and askpass token.
- Key passphrases remain independent. An encrypted selected key may still have a dedicated or named key passphrase.
- Removing a legacy named password assignment must not delete or overwrite the reusable credential or affect its other hosts.
- A save that cleans a legacy key/password conflict must update config, encrypted vault, journal/history, and live vault state together or leave all of them unchanged.
- Raw/Advanced config editing does not become a vault-writing path. Connect-time checks must make any conflict created there inert until Basic save cleans it.
- Secrets must not enter SSH config, response payloads, URL/history, logs, test artifacts, or rendered hidden DOM controls.
- Preserve the existing generated OpenAPI contract unless a compile-time change proves necessary; this feature changes policy, not request/response shapes.
- Refresh `internal/ui/dist`, run the already-parallel Playwright suite, and run Docker-backed integration before pushing `main`.

---

### Task 1: Define One Direct-Identity Policy in the Application Layer

**Files:**
- Modify: `internal/application/passwordeligibility.go`
- Modify: `internal/application/passwordeligibility_test.go`
- Modify: `internal/application/projection.go`
- Modify: `internal/application/projection_test.go`

**Interfaces:**
- Produce a package-private block helper that returns the first direct non-`none` `IdentityFile` and its source line.
- Produce: `func (s *Service) StoredPasswordAllowed(alias string) (bool, error)`.
- Keep: `func (s *Service) PasswordEligibility(alias string) (PasswordEligibility, error)`, but move `identity_file_configured` from warnings to blockers.

- [ ] **Step 1: Write failing direct-field tests**

  Add table-driven projection/application tests for:

  - no direct `IdentityFile`;
  - one direct inventory path;
  - one direct custom path;
  - multiple direct paths;
  - `IdentityFile none` only;
  - `IdentityFile none` followed by a concrete path;
  - a key inherited from `Host *`;
  - a key inherited from another matching non-primary block;
  - unrelated `CertificateFile`, `IdentityAgent`, and `IdentitiesOnly` directives;
  - duplicate concrete aliases, where the first block in OpenSSH read order is authoritative.

  Assert that only a direct non-`none` value returns `Allowed=false`, and that its notice points to the concrete block's own path and line.

- [ ] **Step 2: Run the focused tests and confirm RED**

  Run:

  ```sh
  go test ./internal/application -run 'Test(DirectIdentity|StoredPasswordAllowed|AConfiguredKey)' -count=1
  ```

  Expected: FAIL because `StoredPasswordAllowed` and direct-versus-inherited classification do not exist, and the current key notice is only a warning.

- [ ] **Step 3: Implement a structural direct-field helper**

  Inspect parsed `config.Block` lines rather than the effective projection. Treat values case-insensitively for the `none` sentinel and report the first concrete value:

  ```go
  func directIdentityFile(file *config.File, block config.Block) (Notice, bool) {
      for index := block.Start; index < block.End; index++ {
          line := file.Lines[index]
          if line.Kind != config.LineDirective || !strings.EqualFold(line.Keyword, "IdentityFile") {
              continue
          }
          for _, value := range line.Values() {
              if strings.TrimSpace(value) != "" && !strings.EqualFold(strings.TrimSpace(value), "none") {
                  return Notice{Code: BlockerIdentityFileConfigured, Line: index + 1, Detail: value}, true
              }
          }
      }
      return Notice{}, false
  }
  ```

  Walk the graph in OpenSSH read order, stop at the first concrete block whose `PrimaryAlias` is the requested alias, attach the displayed source path, and fail closed on graph-read errors. Do not use `effective.Project(...).Value("IdentityFile")`; that cannot distinguish a selected block's field from inheritance.

- [ ] **Step 4: Make eligibility use the shared policy**

  Rename the constant to `BlockerIdentityFileConfigured` while keeping its wire code `identity_file_configured`. Append it to `Blockers`, remove it from `Warnings`, and set `Storable` from the final blocker list. Have `StoredPasswordAllowed` validate the alias, resolve the graph, and call the same direct-field classifier.

- [ ] **Step 5: Verify and commit**

  Run:

  ```sh
  go test ./internal/application -count=1
  git diff --check
  git add internal/application
  git commit -m "feat: define explicit key password policy"
  ```

  Expected: PASS; inherited keys and `IdentityFile none` remain eligible, while direct concrete keys are blockers.

---

### Task 2: Add a No-op-safe Config/Vault Transaction Boundary

**Files:**
- Modify: `internal/secret/service.go`
- Modify: `internal/secret/service_test.go`

**Interfaces:**
- Produce:

  ```go
  func (s *Service) WithConnectionSecretsTransaction(
      mutation ConnectionSecretsMutation,
      commit func(vaultChange *storage.Change) (storage.Result, error),
  ) (storage.Result, error)
  ```

- Preserve the existing `WithConnectionSecretsMutation` behavior for existing callers: a semantic no-op still returns `ErrNoPasswordMutation` and does not call their commit callback.

- [ ] **Step 1: Write failing transaction tests**

  Cover all of these cases:

  - removing an absent password calls the new callback once with `nil` while the vault mutation mutex is held;
  - a concurrent password writer cannot pass the transaction until that callback returns;
  - removing a dedicated password supplies one encrypted vault change and deletes only that value;
  - removing a reusable assignment supplies one encrypted vault change and preserves the credential plus its other uses;
  - callback failure publishes neither the clone nor its sealed baseline;
  - a locked existing vault returns `ErrLocked` before the callback;
  - no vault plus a removal-only mutation permits a `nil`-change callback, because there is nothing to clean;
  - no vault plus a password addition or key-passphrase mutation still returns `ErrNoVault`;
  - the old wrapper retains its current no-op result.

- [ ] **Step 2: Run the focused tests and confirm RED**

  Run:

  ```sh
  go test ./internal/secret -run 'TestWithConnectionSecrets(Transaction|Mutation)' -count=1
  ```

  Expected: compile failures for the new transaction method.

- [ ] **Step 3: Implement the new method without weakening publication order**

  Refactor the existing clone/seal/publish implementation so the new method:

  1. acquires `mutationMu`;
  2. determines locked-versus-absent vault state;
  3. applies password and key-passphrase mutations to a clone when present;
  4. invokes the callback with `nil` for a semantic no-op, without sealing;
  5. invokes it with a prepared encrypted change when mutated;
  6. publishes `s.vault`, `s.baseline`, and `s.used` only after callback success.

  Keep `mu` released while the callback enters storage, as the existing backup sealing path can call back into this service. The callback must still execute while `mutationMu` is held so standalone vault writers cannot overtake the config commit.

- [ ] **Step 4: Re-express the old wrapper through the new method**

  Adapt `WithConnectionSecretsMutation` by rejecting a `nil` change with `ErrNoPasswordMutation` and forwarding a non-nil value to its existing callback. This keeps current tests and callers stable while giving connection updates the stronger primitive.

- [ ] **Step 5: Verify race behavior and commit**

  Run:

  ```sh
  go test ./internal/secret -count=1
  go test -race ./internal/secret -count=1
  git diff --check
  git add internal/secret
  git commit -m "feat: support no-op secret transactions"
  ```

  Expected: PASS with no data race and no vault re-seal for the no-op path.

---

### Task 3: Enforce and Clean the Invariant in Connection Updates

**Files:**
- Modify: `internal/application/connectionupdate.go`
- Modify: `internal/application/connectionupdate_test.go`
- Modify: `internal/httpserver/connections_test.go`

**Interfaces:**
- Extend the internal connection plan with the post-update `explicitIdentityFile` result.
- Normalize `password: unchanged` to a removal only when the resulting concrete block has a direct key.
- Keep the public `UpdateConnectionRequest` union unchanged.

- [ ] **Step 1: Replace the now-invalid combined-auth test and add failing policy tests**

  Replace `TestUpdateConnectionSavesIdentityPasswordAndKeyPassphraseInOneTransaction`, whose key-plus-account-password expectation is intentionally obsolete. Add tests proving:

  - selecting a direct key while adding/replacing/assigning a stored account password rejects the whole request with `ErrPasswordIneligible`;
  - the rejected request changes neither config, vault disk bytes, live vault, nor history;
  - changing a direct key to inherit and assigning a password in the same request succeeds, even when `Host *` supplies an inherited key;
  - `IdentityFile none` plus a password succeeds;
  - a direct custom or multiple key created outside Basic is still classified as explicit;
  - an unchanged password association is automatically removed when the resulting block is explicit;
  - cleanup alone is a real successful transaction;
  - dedicated cleanup deletes only the alias value;
  - reusable cleanup unassigns only the alias and leaves the credential value and other hosts intact;
  - key-passphrase replacement and account-password cleanup land in one transaction;
  - no existing assignment causes no vault re-seal and no journal for a cleanup-only request;
  - locked/unreadable vault state rejects a save that may require cleanup;
  - conflict, passphrase validation failure, storage failure, and external file change publish neither side.

- [ ] **Step 2: Run focused application/HTTP tests and confirm RED**

  Run:

  ```sh
  go test ./internal/application ./internal/httpserver -run 'Test(UpdateConnection|ConnectionUpdate)' -count=1
  ```

  Expected: failures because current code checks eligibility against pre-update config, takes a config-only fast path for `password: unchanged`, and permits key-plus-password saves.

- [ ] **Step 3: Include post-update identity state in planning**

  After applying the requested field edits to the exact base block, parse/classify the resulting block with Task 1's helper. Return a focused plan wrapper:

  ```go
  type connectionUpdatePlan struct {
      transaction          planned
      configChanged        bool
      explicitIdentityFile bool
  }
  ```

  Do not infer this from `request.IdentityFile` alone: an unchanged custom path, multiple directives, and a pre-existing `IdentityFile none` must all be handled correctly.

- [ ] **Step 4: Normalize password policy against the post-update state**

  Before any mutation is published:

  - reject `dedicated_password`, `saved_password`, or `new_shared_password` when `explicitIdentityFile` is true;
  - retain an explicit `remove` request;
  - turn `unchanged` into `remove` when explicit and a vault service exists;
  - when the resulting state is non-explicit, evaluate the ordinary blockers with the direct-key decision overridden by the post-update result, so “inherit key + assign password” is not blocked by stale pre-update eligibility;
  - retain the `PasswordAuthentication no` and unsafe-alias blockers.

  Reject unknown mutation kinds before touching config or vault.

- [ ] **Step 5: Commit through the no-op-safe transaction**

  Any resulting explicit-key save must enter `WithConnectionSecretsTransaction` with a removal mutation, even when `Has(alias)` is currently false. Inside its callback, re-plan against the request base, revalidate the selected key/passphrase, and commit config plus an optional vault change in one storage request. This closes the race where another writer assigns a password between a preliminary `Has` check and the config commit.

  For a `nil` vault change:

  - commit only real config changes;
  - return `ErrNoConnectionUpdate` when config is also unchanged;
  - do not seal, write, or journal the vault.

  Keep the existing config-only fast path only when the post-update state is non-explicit and no password/key-passphrase mutation is requested.

- [ ] **Step 6: Map and verify HTTP behavior**

  Preserve the existing `password_ineligible` connection-update problem response. Assert that stale or direct API clients receive a conflict and never get a partial config save.

- [ ] **Step 7: Verify and commit**

  Run:

  ```sh
  go test ./internal/application ./internal/httpserver -count=1
  go test -race ./internal/application ./internal/secret -count=1
  git diff --check
  git add internal/application internal/httpserver
  git commit -m "feat: exclude passwords from explicit key updates"
  ```

  Expected: PASS, including atomic-failure assertions.

---

### Task 4: Close Standalone Save and Connection-time Bypasses

**Files:**
- Modify: `internal/httpserver/password.go`
- Modify: `internal/httpserver/password_test.go`
- Modify: `internal/httpserver/connect.go`
- Modify: `internal/httpserver/connect_test.go`
- Modify: `internal/httpserver/diagnostics.go`
- Modify: `internal/httpserver/diagnostics_test.go`
- Modify: `internal/httpserver/server.go`
- Modify: `internal/httpserver/server_test.go`
- Modify: `internal/app/run.go`
- Modify: `internal/app/run_test.go`

**Interfaces:**
- Add optional `PasswordAllowed func(alias string) (bool, error)` injection to CLI-connect and terminal-launch handlers.
- Wrap askpass's existing prompt predicate with a current-config password policy predicate.
- Reuse the existing `password_not_storable` problem and `identity_file_configured` blocker code.

- [ ] **Step 1: Write failing standalone password API tests**

  Assert:

  - `PUT /api/v1/passwords/{alias}` rejects a direct-key host;
  - password-kind `PUT /api/v1/credentials/password/assign` rejects a direct-key host;
  - key-passphrase assignment remains allowed;
  - inherited keys and `IdentityFile none` allow password store/assignment;
  - eligibility read failure returns `config_unreadable` and mutates nothing;
  - unassign/delete operations remain available for cleanup.

- [ ] **Step 2: Factor one HTTP eligibility guard**

  Extract the existing Store logic into a helper that converts application blockers to `password_not_storable`. Call it from Store and only the password-kind AssignCredential path before entering the vault service. Do not apply it to named credential value replacement, key-passphrase operations, or removal.

- [ ] **Step 3: Write failing issue/redeem/launch tests**

  Cover:

  - CLI connect returns no askpass token for a direct-key alias but still returns a normal connection response;
  - terminal launch uses plain `ssh` rather than `LaunchTerminalWithPassword` for a direct-key alias;
  - inherited key and `IdentityFile none` retain askpass behavior;
  - a config-policy read error fails closed to plain SSH, not a failed launch;
  - a token issued while password use is allowed cannot return a password after a direct key is added;
  - the denied redemption consumes the token, so reverting config does not revive it;
  - a direct-key policy does not alter the non-interactive Authentication diagnostic path.

- [ ] **Step 4: Check policy before token issuance**

  In `ConnectHandlers.Connect` and `DiagnosticsHandlers.armed`, require `PasswordAllowed(alias) == true` in addition to the existing vault/helper conditions. Treat `false` and policy errors as an unarmed plain connection so OpenSSH can ask manually.

- [ ] **Step 5: Recheck policy at redemption time**

  Compose the askpass predicate in server wiring:

  ```go
  func passwordAnswerable(
      promptRule func(alias, prompt string) bool,
      allowed func(alias string) (bool, error),
  ) func(alias, prompt string) bool
  ```

  It returns true only when the prompt rule accepts and the current config policy succeeds with `allowed=true`. `secret.Service.Redeem` already removes the pending token before calling the predicate, which supplies the required issue-to-redeem TOCTOU protection without exposing the secret or adding a second token API.

- [ ] **Step 6: Wire the application service everywhere**

  Supply `options.Config.StoredPasswordAllowed` to connect, terminal, password store/assignment, and askpass composition. Preserve permissive nil behavior only in isolated handler tests/builds that intentionally have no config service; production wiring must always supply the policy.

- [ ] **Step 7: Verify and commit**

  Run:

  ```sh
  go test ./internal/httpserver ./internal/app ./internal/secret -count=1
  go test -race ./internal/httpserver ./internal/app -count=1
  git diff --check
  git add internal/httpserver internal/app
  git commit -m "feat: stop password automation for explicit keys"
  ```

  Expected: PASS; direct-key connections still launch, but receive no saved password at either token boundary.

---

### Task 5: Make Connections Render One Authentication Method

**Files:**
- Create: `web/src/connections/authenticationPolicy.ts`
- Create: `web/src/connections/authenticationPolicy.test.ts`
- Modify: `web/src/connections/ConnectionBasicForm.tsx`
- Modify: `web/src/connections/ConnectionBasicForm.test.tsx`
- Modify: `web/src/connections/connectionSavedState.ts`
- Modify: `web/src/connections/connectionSavedState.test.ts`
- Modify: `web/src/connections/ConnectionSummary.tsx`
- Modify: `web/src/connections/ConnectionSummary.test.tsx`
- Modify: `web/src/diagnostics/PasswordPanel.tsx`
- Modify: `web/src/diagnostics/PasswordPanel.test.tsx`
- Modify: `web/src/i18n/messages.ts`

**Interfaces:**
- Produce small pure helpers for `IdentityFile none` normalization and saved/draft explicit-key state.
- Keep the API request union unchanged; the form sends `{kind: "remove"}` for a visible legacy cleanup.
- Add localized copy for “stored password is unused and will be unassigned on save.”

- [ ] **Step 1: Write failing pure-state and summary tests**

  Assert that:

  - no field and `IdentityFile none` summarize as no explicit key;
  - inventory, custom, unavailable, and multiple concrete values summarize as explicit;
  - an inherited effective key does not change the direct state;
  - ordinary explicit-key summaries omit the account-password row;
  - an explicit-key summary with a dedicated or named legacy assignment shows only the cleanup warning, never an active-password label;
  - a named cleanup warning does not imply the reusable credential will be deleted;
  - no-key summaries retain the current password state row.

- [ ] **Step 2: Run focused UI tests and confirm RED**

  Run:

  ```sh
  cd web && npm test -- --run \
    src/connections/authenticationPolicy.test.ts \
    src/connections/connectionSavedState.test.ts \
    src/connections/ConnectionSummary.test.tsx
  ```

  Expected: compile/test failures for the helper and current always-visible account-password row.

- [ ] **Step 3: Centralize browser-side direct-key classification**

  Normalize direct `IdentityFile` fields with the same rule as Go: ignore empty/`none` values, treat any remaining direct value as explicit, and retain a separate “multiple/custom cannot be edited in Basic” state. Fix the current `IdentityFile none` initialization so it selects “SSH agent or inherited key” rather than a custom key.

- [ ] **Step 4: Write failing Basic-form interaction tests**

  Cover:

  - selecting a known key removes account-password controls from the DOM and clears typed dedicated/new-shared secrets;
  - custom and multiple direct keys also hide those controls;
  - selecting agent/inherited restores empty password controls without restoring cleared secret input;
  - an existing direct-key conflict marks the form dirty and enables save with no other edit;
  - that save sends `password: {kind: "remove"}` without a confirmation checkbox;
  - reverting to the saved no-key choice cancels cleanup;
  - choosing a direct key while a password is assigned sends key set plus password removal together;
  - changing a saved direct key to inherited permits password entry/assignment in the same request;
  - the stale `identity_file_configured` eligibility blocker is ignored only when the draft removes the direct key; other blockers still disable save;
  - unencrypted known keys show one short “no passphrase needed” sentence without the saved-passphrase heading;
  - encrypted keys retain dedicated/named/no-passphrase state and replacement inputs;
  - custom/multiple keys keep the Advanced guidance and do not offer key-passphrase editing;
  - discard, navigation, and failed save do not change the vault and clear secret drafts.

- [ ] **Step 5: Render mutually exclusive form sections**

  Derive `draftHasExplicitKey` from current selection plus direct custom/complex state. When it becomes true:

  - clear password secret fields and return the action selector to `unchanged`;
  - hide the stored-account-password state, vault unlock prompt, action selector, inputs, and eligibility notices from the DOM;
  - if `assigned && draftHasExplicitKey`, show only cleanup metadata and derive an automatic remove mutation;
  - count cleanup as dirty and require an unlocked/readable vault before save;
  - keep selected known-key passphrase controls independent.

  When `draftHasExplicitKey` becomes false, restore the ordinary account-password section with blank secret fields. Derive password eligibility from all non-key blockers so a single request can inherit the key and assign a password.

- [ ] **Step 6: Simplify summary and copy**

  Render the account-password row only for no-key connections. For explicit-key conflicts, render a danger/notice row saying the assignment is unused and Basic save will remove only the association. Remove use of the “keys and stored passwords are independent” copy and do not show the old key warning in Basic. Update the shared eligibility-code mapping so `identity_file_configured` has blocker wording; retain the unmounted legacy `PasswordPanel` tests as another check that a stale caller cannot offer assignment controls for a direct-key host.

- [ ] **Step 7: Verify and commit**

  Run:

  ```sh
  cd web && npm test -- --run src/connections src/i18n/i18n.test.tsx
  cd web && npm run typecheck
  git diff --check
  git add web/src
  git commit -m "feat: simplify connection authentication controls"
  ```

  Expected: PASS, with no password input present in the rendered tree while a direct key is selected.

---

### Task 6: Prove the User Flow and Real SSH Boundary

**Files:**
- Modify: `web/e2e/connections.spec.ts`
- Modify: `web/e2e/password.spec.ts`
- Modify: `internal/sshintegration/askpass_test.go`
- Modify: `internal/httpserver/integration_test.go` if the full CLI route is best exercised there
- Regenerate: `internal/ui/dist/**`

**Interfaces:**
- No new product interface; this task exercises the complete browser/API/askpass flow.

- [ ] **Step 1: Add failing browser journeys**

  Add independent Playwright tests (safe for `fullyParallel`) that prove:

  - a no-key connection can assign and display a stored password;
  - selecting a key hides password controls, exposes only the relevant passphrase state, and saves cleanup;
  - reopening the connection shows no active account-password state;
  - switching the key back to agent/inherited allows a new password assignment;
  - direct custom/multiple fields are treated as explicit and routed to Advanced guidance;
  - the URL and selected Basic panel remain stable through save/refresh.

- [ ] **Step 2: Run the affected E2E files and confirm RED, then GREEN**

  Run:

  ```sh
  cd web && npx playwright test e2e/connections.spec.ts e2e/password.spec.ts --workers=4
  ```

  Expected before implementation: policy assertions fail. Expected after Tasks 1–5: PASS. Keep trace/video/screenshot disabled so secret-bearing screens do not produce artifacts.

- [ ] **Step 3: Add a Docker-backed askpass policy assertion**

  Against the pinned OpenSSH container, prove both sides:

  - a no-direct-key or `IdentityFile none` fixture with a stored password can authenticate via the one-time askpass token;
  - the same stored legacy association with a direct key cannot obtain/redeem a usable token, while the generated SSH invocation itself remains an ordinary interactive-capable command.

  Do not try to automate a human prompt or weaken sshd settings beyond the existing integration fixture.

- [ ] **Step 4: Refresh embedded assets**

  Run:

  ```sh
  make build
  git status --short internal/ui/dist
  ```

  Expected: Vite/typecheck succeeds and only the current hashed production assets plus manifest/index changes are present.

- [ ] **Step 5: Verify and commit the end-to-end slice**

  Run:

  ```sh
  git diff --check
  git add web/e2e internal/sshintegration internal/httpserver/integration_test.go internal/ui/dist
  git commit -m "test: cover explicit key password exclusion"
  ```

  Add only paths that actually changed; do not stage an unchanged integration file.

---

### Task 7: Full Verification, Review, and Push

**Files:**
- Review all files changed by Tasks 1–6.
- Do not modify unrelated dirty files if any appear.

- [ ] **Step 1: Run generated-contract and deterministic suites**

  Run:

  ```sh
  make verify-generated
  make test
  ```

  Expected: PASS. `verify-generated` should report no API artifact drift; `make test` includes Go tests, Go race tests, Vitest, and TypeScript checks.

- [ ] **Step 2: Run the parallel browser suite**

  Run:

  ```sh
  make e2e
  ```

  Expected: PASS using the repository's existing local `workers: 4` and `fullyParallel: true` configuration.

- [ ] **Step 3: Run Docker integration with guaranteed cleanup**

  Run:

  ```sh
  make integration-up
  make integration
  make integration-down
  ```

  If integration fails, collect the focused test output and container logs before `make integration-down`; do not leave `sshc-s3`, `sshc-sshd`, or `.integration-s3.json` behind.

  Expected: real S3 and OpenSSH tests PASS, including the new saved-password exclusion assertion.

- [ ] **Step 4: Perform final invariant review**

  Inspect the final diff and explicitly verify:

  - every password creation/assignment route is blocked for direct keys;
  - every automatic-use route checks at issue and redeem time;
  - no normal connection disables OpenSSH manual prompts;
  - named credential cleanup is association-only;
  - `IdentityFile none` and inherited identities remain allowed;
  - cleanup failure cannot leave config and vault divergent;
  - no secret value appears in config, HTTP response, UI copy, URL, logs, or committed assets.

  Run:

  ```sh
  git status --short --branch
  git diff --stat origin/main...HEAD
  git log --oneline --decorate origin/main..HEAD
  ```

- [ ] **Step 5: Push the verified branch**

  Run:

  ```sh
  git push origin main
  git status --short --branch
  ```

  Expected: push succeeds and `main` is synchronized with `origin/main`; if the remote advanced, stop and inspect rather than forcing.
