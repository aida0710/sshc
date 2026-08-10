# Connection Key Passphrase and Settings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Store and replace a selected encrypted key's non-reusable passphrase from Connection Basic settings, and move launch-at-login and master-password rotation into `/settings`.

**Architecture:** Extend vault schema v3 with a key-owned map parallel to dedicated host passwords, and generalise the existing cloned-vault connection transaction so SSH config, account passwords, and key passphrases publish atomically. The connection form sends a discriminated key-passphrase mutation in its existing save request; a new Settings panel reuses the current login-item and master-password APIs.

**Tech Stack:** Go 1.25, Echo v5, OpenAPI/oapi-codegen, React 19, TypeScript, Vitest, Playwright, Docker Compose-style integration targets in Make.

## Global Constraints

- Do not install packages; use the repository's existing Go and npm lockfiles.
- A dedicated key passphrase must be structurally non-reusable and change only its private-key subject.
- Never return or persist plaintext passphrases in responses, URLs, browser storage, logs, history, previews, or diffs.
- A failed validation or disk commit leaves config bytes, sealed vault bytes, and live vault state unchanged.
- `Save Basic settings` is the only Connection action that persists a typed key passphrase.
- The saved unlock value does not change the private-key file's own encryption passphrase.
- Launch-at-login stays default-off; master-password rotation and snapshot resealing keep their existing semantics.

## File map

| Responsibility | Files |
| --- | --- |
| Vault v3 and dedicated-key invariants | `internal/secret/vault.go`, `internal/secret/vault_test.go` |
| Atomic cloned-vault mutation and status | `internal/secret/service.go`, `internal/secret/service_test.go` |
| Key passphrase verification | `internal/keys/service.go`, `internal/keys/service_test.go` |
| Connection update domain and atomic commit | `internal/application/connectionupdate.go`, `internal/application/connectionupdate_test.go` |
| HTTP decoding and problem mapping | `internal/httpserver/connections.go`, `internal/httpserver/connections_test.go`, `internal/httpserver/password.go`, `internal/httpserver/password_test.go` |
| API source and generated clients | `api/openapi.yaml`, `internal/api/models.gen.go`, `web/src/api/schema.d.ts` |
| Connection authentication UI | `web/src/connections/ConnectionBasicForm.tsx`, `web/src/connections/ConnectionBasicForm.test.tsx`, `web/src/api/integrations.ts` |
| Keys-screen dedicated status | `web/src/keys/KeysScreen.tsx`, `web/src/keys/KeysScreen.test.tsx` |
| Settings route and panel | `web/src/settings/SettingsPanel.tsx`, `web/src/settings/SettingsPanel.test.tsx`, `web/src/secrets/SecretsPanel.tsx`, `web/src/secrets/SecretsPanel.test.tsx`, `web/src/routing/sectionRoute.ts`, `web/src/routing/sectionRoute.test.ts`, `web/src/App.tsx`, `web/src/App.test.tsx`, `web/src/ui/icons.tsx`, `web/src/ui/icons.test.tsx`, `web/src/i18n/messages.ts` |
| Browser workflows | `web/e2e/connections.spec.ts`, `web/e2e/secrets.spec.ts`, `web/e2e/routing.spec.ts`, `web/e2e/keys.spec.ts` |

---

### Task 1: Vault schema v3 and key-owned passphrases

**Interfaces:**

- Produces: `(*Vault).SetDedicatedKeyPassphrase(relativePath, value string) error`
- Produces: `(*Vault).RemoveKeyPassphrase(relativePath string)`
- Produces: `(*Vault).DedicatedKeyPassphraseSubjects() []string`
- Changes: `SecretFor(KindKeyPassphrase, subject)` resolves the dedicated map first.
- Changes: `Assign(KindKeyPassphrase, subject, name)` removes the subject's dedicated value.
- Changes: `RelocateSubjects(KindKeyPassphrase, relocations)` relocates both representations.

- [ ] **Step 1: Write failing vault tests**

Add literal assertions covering v2 open → v3 seal, an older-reader-safe version boundary, dedicated set/replace, named↔dedicated transitions, removal, resolution, clone isolation, and relocation. The core one-key isolation assertion is:

```go
_ = vault.Set(secret.KindKeyPassphrase, "shared", "old")
_ = vault.Assign(secret.KindKeyPassphrase, "id_a", "shared")
_ = vault.Assign(secret.KindKeyPassphrase, "id_b", "shared")
_ = vault.SetDedicatedKeyPassphrase("id_a", "new")
if got, _ := vault.SecretFor(secret.KindKeyPassphrase, "id_a"); got != "new" { t.Fatal(got) }
if got, _ := vault.SecretFor(secret.KindKeyPassphrase, "id_b"); got != "old" { t.Fatal(got) }
```

- [ ] **Step 2: Verify RED**

Run `go test ./internal/secret -run 'Test.*DedicatedKey|Test.*Version3' -count=1` and confirm failures name the missing methods/schema behavior.

- [ ] **Step 3: Implement the minimal vault model**

Add `DedicatedKeyPassphrases map[string]string \`json:"dedicatedKeyPassphrases,omitempty"\`` to `document`, a cloned map to `Vault`, schema version 3 with explicit version-2 migration, and the four invariants above. Version 1 remains `ErrOldVault`; versions above 3 remain `ErrUnsupportedVersion`.

- [ ] **Step 4: Verify GREEN and the package**

Run `go test ./internal/secret -count=1`.

- [ ] **Step 5: Commit**

Commit `feat: add dedicated key passphrases to vault`.

---

### Task 2: One atomic connection-secret mutation

**Interfaces:**

- Produces:

```go
type KeyPassphraseMutation struct {
    RelativePath string
    Passphrase   string
}
type ConnectionSecretsMutation struct {
    Password      *PasswordMutation
    KeyPassphrase *KeyPassphraseMutation
}
func (s *Service) WithConnectionSecretsMutation(
    mutation ConnectionSecretsMutation,
    commit func(storage.Change) (storage.Result, error),
) (storage.Result, error)
```

- Keeps: `WithPasswordMutation` as a compatibility wrapper for connection creation.
- Produces: `DedicatedKeyPassphrases() []string` on `Service`, exposed only while unlocked.

- [ ] **Step 1: Write failing service tests**

Cover dedicated-only mutation, password+key mutation in one sealed write, shared-to-dedicated isolation, same-value no-op, locked/no-vault errors, commit failure preserving disk and live memory, rekeyed-baseline use, and relocation of dedicated values.

- [ ] **Step 2: Verify RED**

Run `go test ./internal/secret -run 'Test.*ConnectionSecrets|Test.*DedicatedKeyPassphrase' -count=1` and confirm the missing API/invariants cause the failures.

- [ ] **Step 3: Implement the generalised clone transaction**

Under `mutationMu`, clone once, apply the optional account-password mutation and optional dedicated-key mutation, seal once, call the commit callback once, then publish clone/baseline only on success. Compare existing dedicated values in constant time. Do not expose a generic API that can assign a dedicated value to another subject.

- [ ] **Step 4: Extend status and relocation**

Make `DedicatedKeyPassphrases` return sorted relative paths; make `RelocateKeyPassphrases` move both the named subject map and the dedicated map in the same sealed write.

- [ ] **Step 5: Verify GREEN and race-sensitive package behavior**

Run `go test ./internal/secret -count=1` and `go test -race ./internal/secret -count=1`.

- [ ] **Step 6: Commit**

Commit `feat: transact connection secrets together`.

---

### Task 3: Validate the chosen encrypted key and bind it to the connection

**Interfaces:**

- Produces:

```go
type PassphraseVerification struct {
    KeyID        string
    RelativePath string
    Digest       string
}
func (service *Service) VerifyPassphrase(keyID string, passphrase []byte) (PassphraseVerification, error)
func (service *Service) RevalidatePassphrase(verification PassphraseVerification) error
```

- Produces in `application`:

```go
type UpdateConnectionKeyPassphraseKind string
const (
    UpdateKeyPassphraseUnchanged    UpdateConnectionKeyPassphraseKind = "unchanged"
    UpdateKeyPassphraseSetDedicated UpdateConnectionKeyPassphraseKind = "set_dedicated"
)
type UpdateConnectionKeyPassphrase struct {
    Kind       UpdateConnectionKeyPassphraseKind
    KeyID      string
    Passphrase string
}
```

- [ ] **Step 1: Write failing key-service tests**

Use real in-process encrypted fixtures to assert correct passphrase success, wrong passphrase `ErrWrongPassphrase`, unencrypted refusal, missing/non-private refusal, byte-digest evidence, and changed-byte revalidation failure. Assert submitted byte slices are wiped before return.

- [ ] **Step 2: Verify key-service RED**

Run `go test ./internal/keys -run 'Test.*VerifyPassphrase' -count=1`.

- [ ] **Step 3: Implement key verification**

Resolve the key only from inventory ID, read workspace bytes, call `DecodePrivateKey`, derive a digest from those exact bytes, wipe the request slice, and re-read/re-hash for revalidation. Return no private material or passphrase.

- [ ] **Step 4: Write failing application tests**

Add cases for passphrase-only save, simultaneous `IdentityFile`+dedicated passphrase save, simultaneous account password+key passphrase save, wrong/unrelated/unencrypted/custom/complex key refusal, named shared passphrase preservation for another key, stale config, changed key bytes, injected first/second file commit failures, and successful status refresh.

- [ ] **Step 5: Verify application RED**

Run `go test ./internal/application -run 'TestUpdateConnection.*Passphrase' -count=1`.

- [ ] **Step 6: Implement the connection mutation path**

Extend `UpdateConnectionRequest`. Derive the resulting direct `IdentityFile` from the planned file and require it to equal `~/.ssh/<verified relativePath>`. When either secret changes, call `WithConnectionSecretsMutation`; append exactly one sealed-vault change to the existing atomic storage request. Revalidate key evidence immediately before committing. Preserve the config-only fast path.

- [ ] **Step 7: Verify GREEN**

Run `go test ./internal/keys ./internal/application -count=1`.

- [ ] **Step 8: Commit**

Commit `feat: save connection key passphrases atomically`.

---

### Task 4: Extend the OpenAPI and HTTP boundary

**Interfaces:**

- `PasswordVaultStatus` adds required `dedicatedKeyPassphrases: string[]`.
- `UpdateConnectionRequest` adds required `keyPassphrase`, a discriminated union of `unchanged` and `set_dedicated`.
- Problem codes: `wrong_passphrase` (403), `identity_file_invalid` (422), `external_change` (409), `invalid_request` (400), and existing vault/commit problems.

- [ ] **Step 1: Write failing HTTP tests**

Decode both union members, reject unknown/extra/missing fields, prove a submitted secret never appears in success/problem response bodies, map wrong passphrase and stale evidence correctly, and include only dedicated relative paths in vault status.

- [ ] **Step 2: Verify HTTP RED**

Run `go test ./internal/httpserver -run 'Test(UpdateConnection|PasswordVault).*Passphrase' -count=1`.

- [ ] **Step 3: Update the API source and handlers**

Define `UpdateConnectionKeyPassphrase`, `ConnectionKeyPassphraseUnchanged`, and `ConnectionKeyPassphraseSetDedicated` in `api/openapi.yaml`; decode them in `connections.go`; include status paths in `password.go`; keep response bodies value-free.

- [ ] **Step 4: Regenerate committed models**

Run `make generate`. Do not hand-edit `internal/api/models.gen.go` or `web/src/api/schema.d.ts`.

- [ ] **Step 5: Verify generated files and GREEN**

Run `go test ./internal/httpserver -count=1` and `make verify-generated`.

- [ ] **Step 6: Commit**

Commit `feat: expose dedicated key passphrase updates`.

---

### Task 5: Add the inline Connection passphrase editor

**Interfaces:**

- Consumes `PasswordVaultStatus.dedicatedKeyPassphrases` and named `Credential.uses`.
- Sends:

```ts
keyPassphrase: passphrase === ""
  ? { kind: "unchanged" }
  : { kind: "set_dedicated", keyId: selected.id, passphrase }
```

- [ ] **Step 1: Write failing component tests**

Name and cover these observable breaks: no editor for no/custom/complex key; no input for unencrypted key; unsaved/shared/dedicated copy; shared uses remain visible; mismatch disables save; correct pair adds one mutation to the existing request; selected-key change clears both secret fields; success/failure clears them; stable server problems appear beside the editor; passphrase-only save is enabled.

- [ ] **Step 2: Verify component RED**

Run `npm test --prefix web -- ConnectionBasicForm.test.tsx` and confirm failures are due to missing controls/request fields.

- [ ] **Step 3: Implement minimal form state and rendering**

Track `keyPassphrase` and `keyPassphraseConfirmation`; derive the selected `KeyItem`, named assignment, and dedicated status without reading any value. Add the editor below the key selector, clear it in `clearSecrets` and on key changes, include the mutation in `dirty`/`canSave`, and never render the submitted text outside password inputs.

- [ ] **Step 4: Update API validation fixtures**

Require and validate `dedicatedKeyPassphrases` as a string array in `web/src/api/integrations.ts`; update every `PasswordVaultStatus` fixture with `[]` or the intended relative paths.

- [ ] **Step 5: Verify GREEN and mutation check**

Run `npm test --prefix web -- ConnectionBasicForm.test.tsx integrations.test.ts`. Mentally mutate selected key ID, shared/dedicated branch, and field-clearing calls; ensure a named test fails for each.

- [ ] **Step 6: Commit**

Commit `feat: edit key passphrases from connections`.

---

### Task 6: Keep Keys consistent with dedicated values

**Interfaces:**

- Keys loads `passwordVault()` with `credentials()` when a passphrase action opens.
- A dedicated subject is reported as `Saved for this key` and `unassignCredential("key_passphrase", relativePath)` removes it through the server's unified removal semantics.

- [ ] **Step 1: Write failing KeysScreen tests**

Add dedicated-state copy, agent-add stored-value hint, dedicated detach, named picker replacing a dedicated value for one key, and secret-not-in-DOM assertions.

- [ ] **Step 2: Verify RED**

Run `npm test --prefix web -- KeysScreen.test.tsx`.

- [ ] **Step 3: Implement dedicated status handling**

Extend the injected Secrets API pick with `passwordVault`, load paths only on demand, and make `storedFor`/rendering distinguish named from dedicated without synthesising a reusable name.

- [ ] **Step 4: Verify GREEN**

Run `npm test --prefix web -- KeysScreen.test.tsx`.

- [ ] **Step 5: Commit**

Commit `fix: show dedicated passphrases on keys`.

---

### Task 7: Add `/settings` and move both settings cards

**Interfaces:**

- `Section` gains `Settings`; `sectionPath("Settings") === "/settings"`.
- `SettingsPanel({ api?: IntegrationsApi })` owns launch-at-login and master-password state.
- `SecretsPanel` retains `onLock` and credential management only.

- [ ] **Step 1: Write failing routing and shell tests**

Assert Settings is in the route table, has `/settings`, appears in Maintenance with a settings icon, direct/back/forward navigation works, and Secrets no longer contains either moved region.

- [ ] **Step 2: Write failing SettingsPanel tests**

Move and strengthen the current behaviors: supported on/off toggle, unsupported explanation/omission as chosen by existing contract, load/save failure notices, current/new/confirmation validation, wrong-current handling, successful snapshot-resealed and local-only result messages, and clearing all master-password inputs on success/failure.

- [ ] **Step 3: Verify RED**

Run `npm test --prefix web -- sectionRoute.test.ts App.test.tsx SettingsPanel.test.tsx SecretsPanel.test.tsx`.

- [ ] **Step 4: Implement the route, icon, panel, and move**

Create `SettingsPanel.tsx`, move `LoginItemSection` and `changeMaster` state out of `SecretsPanel`, add English/Japanese copy, wire `Settings` through labels/icons/navigation/`PaddedSection`, and add the gear symbol to the closed icon union.

- [ ] **Step 5: Verify GREEN and typecheck**

Run the targeted tests, then `npm run typecheck --prefix web`.

- [ ] **Step 6: Commit**

Commit `feat: move application controls to settings`.

---

### Task 8: Browser workflows, full verification, review, and push

**Interfaces:** None; this task proves the end-user contract and publishes it.

- [ ] **Step 1: Write failing E2E workflows**

Add connection coverage that selects an encrypted key, saves a dedicated value, replaces it, and sees only key-owned status. Extend key coverage to prove agent registration works with the saved value and another key's shared assignment survives. Move login/master interactions from `/secrets` to `/settings`; add direct Settings routing.

- [ ] **Step 2: Verify E2E RED**

Run the smallest matching Playwright specs with `npm run e2e --prefix web -- connections.spec.ts keys.spec.ts secrets.spec.ts routing.spec.ts` and confirm only the new assertions fail before their fixtures/behavior are finished.

- [ ] **Step 3: Complete E2E fixtures and make them GREEN**

Use only repository-created temporary homes and test passphrases. Never print or attach a real local vault or private key.

- [ ] **Step 4: Run fresh full verification**

Run, in order:

```text
make test
make e2e
make integration-up
make integration
make integration-down
make verify-generated
```

Use a cleanup trap around the three integration commands so containers and `.integration-s3.json` are removed after success or failure. Confirm `docker ps` contains neither `sshc-s3` nor `sshc-sshd` afterward.

- [ ] **Step 5: Review security and UX invariants**

Inspect the final diff for secret response/DOM/history leaks, partial commits, stale-key races, Settings duplication, inaccessible labels, and shared-credential mutation. Fix every Critical or Important finding with a new failing regression test before code.

- [ ] **Step 6: Commit final test/review fixes**

Commit `test: cover connection key passphrase workflow` or a narrower fix message matching the actual diff.

- [ ] **Step 7: Push and watch CI**

Confirm a clean worktree, push `main`, watch the exact GitHub Actions run to completion, and verify `HEAD == origin/main` at the pushed commit before reporting completion.

## Plan self-review

- Spec coverage: vault v3 migration, structural non-reuse, atomic combined save, key validation, every UI state, Settings movement, Keys consistency, and Docker/CI verification each have a task.
- Placeholder scan: every step names its concrete behavior, command, and expected boundary.
- Type consistency: `KeyPassphraseMutation` is the secret-layer type; `UpdateConnectionKeyPassphrase` is the application/API type; `dedicatedKeyPassphrases` is the single wire-field spelling in Go-generated and TypeScript clients.
- Execution: inline in the current session with `superpowers:executing-plans`; no subagent dispatch is required or authorised.
