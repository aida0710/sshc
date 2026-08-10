# Connection Basic Editing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the existing directive-only Basic tab with an always-present connection and authentication form that safely updates sparse or inherited SSH settings and encrypted password assignments.

**Architecture:** Add a typed `PATCH /api/v1/connections` use case. The server derives line edits from semantic field operations and combines password-vault mutations with configuration writes in one storage transaction. A focused React `ConnectionBasicForm` derives direct/inherited/default display state from `HostDetail`, loads key and vault options, and submits only explicit changes while the existing detail shell retains advanced editors and previews.

**Tech Stack:** Go 1.24, Echo v5, oapi-codegen, OpenAPI 3, React 19, TypeScript, Vite, Vitest/Testing Library, Playwright, Tailwind CSS.

## Global Constraints

- Do not install or add a package; dependency manifests and lockfiles must remain unchanged.
- Never expose plaintext passwords or sealed-vault bytes in SSH config, preview, response, logs, browser history or a prefilled DOM value.
- Opening or saving an unchanged form must not materialise inherited/default values into the selected Host block.
- Basic must reject duplicate direct HostName, User, Port or IdentityFile values rather than flattening them.
- Password `unchanged` must work with an absent or locked vault; any other password mutation requires an existing unlocked vault.
- Configuration and vault changes in one request must commit atomically or leave disk and in-memory state unchanged.
- Preserve unrelated configuration bytes, comments, line endings, unknown directives and Host blocks.
- Keep alias rename, group move and comment editing in Organisation; keep complex/custom authentication in Advanced.
- Use TDD for every handwritten behavior change and run the named failing test before production implementation.

---

### Task 1: Update contract and removable password mutation

**Files:**
- Modify: `api/openapi.yaml`
- Modify: `internal/api/contract_test.go`
- Generated: `internal/api/models.gen.go`
- Generated: `web/src/api/schema.d.ts`
- Modify: `internal/secret/service_test.go`
- Modify: `internal/secret/service.go`

**Interfaces:**
- Produces: OpenAPI `UpdateConnectionRequest`, `ConnectionStringChange`, `ConnectionPortChange`, `ConnectionIdentityFileChange`, and `UpdateConnectionPassword` unions.
- Produces: `secret.PasswordMutationRemove` accepted by `(*secret.Service).WithPasswordMutation`.
- Consumes: existing `HostIdentity`, `SaveResult`, `Create*PasswordAuthentication`, `storage.Change`, and vault cloning primitives.

- [ ] **Step 1: Write failing generated-contract and vault-removal tests**

Add `TestGeneratedConnectionUpdateModels` to `internal/api/contract_test.go`. It must compile against the wished-for generated shape:

```go
func TestGeneratedConnectionUpdateModels(t *testing.T) {
	request := UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"},
		Base: "Host edge\n",
		HostName: pointerToRaw(json.RawMessage(`{"action":"set","value":"edge.example"}`)),
		Password: json.RawMessage(`{"kind":"unchanged"}`),
	}
	if request.Identity.Alias != "edge" || request.HostName == nil || len(request.Password) == 0 {
		t.Fatalf("unexpected connection update contract: %#v", request)
	}
}
```

Use a test-local `pointerToRaw(value json.RawMessage) *json.RawMessage` helper;
optional raw unions generate as `*json.RawMessage`, while the required password
union generates as `json.RawMessage`.

Extend the password-mutation table in `internal/secret/service_test.go` with a remove case. Prepare both a dedicated password and a reusable password assignment, call:

```go
_, err := service.WithPasswordMutation(secret.PasswordMutation{
	Kind: secret.PasswordMutationRemove,
	Alias: "edge-remove",
}, commit)
```

and assert `PasswordFor("edge-remove") == ""`, the reusable credential remains listed, and a failed commit publishes neither removal to memory nor new sealed bytes to disk. Add a separate test asserting removal of an alias with no assigned password returns `secret.ErrNoPassword` without invoking the callback.

- [ ] **Step 2: Run the focused tests and verify RED**

Run:

```bash
go test ./internal/api ./internal/secret -run 'TestGeneratedConnectionUpdateModels|TestPasswordMutationsCommitEachSupportedSource|TestPasswordMutationRemove' -count=1
```

Expected: compilation fails because `UpdateConnectionRequest` and `PasswordMutationRemove` do not exist.

- [ ] **Step 3: Define the OpenAPI update request**

Add `patch` beside the existing `post` operation at `/api/v1/connections`, with operation ID `updateConnection`, request `UpdateConnectionRequest`, response `SaveResult`, and Problem responses for 400, 401, 403, 404, 409, 422 and 500.

Define strict schemas with `additionalProperties: false`:

```yaml
UpdateConnectionRequest:
  type: object
  additionalProperties: false
  required: [identity, base, password]
  properties:
    identity: { $ref: "#/components/schemas/HostIdentity" }
    base: { type: string, maxLength: 1048576 }
    hostName: { $ref: "#/components/schemas/ConnectionStringChange" }
    user: { $ref: "#/components/schemas/ConnectionStringChange" }
    port: { $ref: "#/components/schemas/ConnectionPortChange" }
    identityFile: { $ref: "#/components/schemas/ConnectionIdentityFileChange" }
    password: { $ref: "#/components/schemas/UpdateConnectionPassword" }
```

Make each change union `x-go-type: json.RawMessage`. Its `set` and `inherit` branches are strict objects. String set has `{action:"set", value:string}`; Port set has an integer value from 1 to 65535; IdentityFile set has a 32-character `keyId`. `UpdateConnectionPassword` is a raw discriminated union with `unchanged`, the three existing create password branches, and strict `{kind:"remove"}`.

- [ ] **Step 4: Generate models and verify the contract test passes**

Run:

```bash
make generate
go test ./internal/api -run TestGeneratedConnectionUpdateModels -count=1
```

Expected: PASS and generated Go/TypeScript models contain the discriminated update types.

- [ ] **Step 5: Implement removable cloned-vault mutation**

In `internal/secret/service.go`, add:

```go
const PasswordMutationRemove PasswordMutationKind = "remove"
```

In `WithPasswordMutation`, handle it on the clone only. First require
`clone.SecretFor(KindPassword, mutation.Alias)` to exist; otherwise return
`ErrNoPassword`. Then perform the same semantic cleanup as `Remove`: remove a
dedicated value, unassign a reusable credential, and delete an alias-named
credential only when it is no longer used. Seal and publish the clone through
the existing callback path; do not call the eager-writing `Service.Remove`.

- [ ] **Step 6: Run Task 1 tests and commit**

Run:

```bash
go test ./internal/api ./internal/secret -count=1
make generate
git diff --check
```

Expected: tests pass, a second generation produces no additional model changes,
and the diff is whitespace-clean.

Commit:

```bash
git add api/openapi.yaml internal/api/contract_test.go internal/api/models.gen.go \
  web/src/api/schema.d.ts internal/secret/service.go internal/secret/service_test.go \
  docs/superpowers/plans/2026-08-10-connection-basic-editing-implementation-plan.md
git commit -m "feat: define connection update contract"
make verify-generated
```

Expected: the post-commit generated check passes with no model diff.

---

### Task 2: Atomic application update use case

**Files:**
- Create: `internal/application/connectionupdate.go`
- Create: `internal/application/connectionupdate_test.go`
- Modify: `internal/application/connectioncreate.go`
- Modify: `internal/application/edit.go`

**Interfaces:**
- Consumes: `secret.PasswordMutation`, `keys.Inventory`, `EditRequest`, `ApplyFieldEdits`, `planned`, `requestFor`, `commitCreatedConnectionRequest` behavior.
- Produces: `ConnectionChangeAction`, `ConnectionStringChange`, `ConnectionPortChange`, `ConnectionIdentityFileChange`, `UpdateConnectionPassword`, `UpdateConnectionRequest` and `(*Service).UpdateConnection(*secret.Service, *keys.Inventory, UpdateConnectionRequest) (SaveResult, error)`.
- Produces errors: `ErrNoConnectionUpdate`, `ErrComplexConnectionField`, plus existing validation, key, credential, vault and conflict errors.

- [ ] **Step 1: Write application tests for semantic field updates**

Create a harness modelled on `connectionCreateHarness` with a sparse target,
an inherited wildcard block, an inventoried private key and an unlocked vault.
Write table tests that call the wished-for `UpdateConnection` API and assert:

```go
request := UpdateConnectionRequest{
	Identity: HostIdentity{Path: "config", Alias: "edge"},
	Base: readFile(t, harness.workspace, "config"),
	HostName: &ConnectionStringChange{Action: ConnectionChangeSet, Value: "198.51.100.8"},
	User: &ConnectionStringChange{Action: ConnectionChangeSet, Value: "deploy"},
	Port: &ConnectionPortChange{Action: ConnectionChangeSet, Value: 2222},
	Password: UpdateConnectionPassword{Kind: UpdatePasswordUnchanged},
}
```

Cover add, set and inherit for HostName/User/Port; set and inherit one
IdentityFile by key ID; exact preservation of unrelated bytes; duplicate direct
keyword rejection; unsafe host/user/port; unknown/public/symlink key; stale
base conflict; and an all-unchanged request returning `ErrNoConnectionUpdate`.

- [ ] **Step 2: Run the semantic tests and verify RED**

Run:

```bash
go test ./internal/application -run 'TestUpdateConnection' -count=1
```

Expected: compilation fails because the update types and method do not exist.

- [ ] **Step 3: Add exact direct-field edit derivation**

In `connectionupdate.go`, define the interfaces above. Implement a helper with
the exact contract:

```go
func connectionFieldEdit(file *config.File, block config.Block, keyword string, action ConnectionChangeAction, values []string) (FieldEdit, bool, error)
```

Scan only `config.LineDirective` lines inside the selected block whose keyword
matches case-insensitively. Zero direct values plus `set` returns an `ActionAdd`;
one plus `set` returns `ActionSet` for that one-based line; one plus `inherit`
returns `ActionRemove`; zero plus `inherit` returns no edit; more than one
returns `ErrComplexConnectionField`. Reject structural keywords and invalid
actions. Keep parsing/rendering inside existing config primitives.

Resolve IdentityFile `keyId` through the current inventory using the same
workspace/symlink/private-key checks as creation and translate it to the
workspace-relative `~/.ssh/...` value. Do not accept a path from the request.

- [ ] **Step 4: Implement config-only update and verify GREEN**

`UpdateConnection` must validate identity/base, resolve the graph under
`saveMutex`, locate the exact Host block, derive `FieldEdit`s, call the existing
edit machinery, compare the supplied base with disk, and commit as
`connection.update`. If password is `unchanged`, never inspect `secrets`.
Return `SaveResult` with displayed written paths and the configuration preview.

Extract the conflict-aware commit body currently named
`commitCreatedConnectionRequest` into a neutral `commitPlannedRequest`; keep
creation calling the neutral helper so its behavior stays unchanged.

Run:

```bash
go test ./internal/application -run 'TestUpdateConnection' -count=1
```

Expected: semantic config tests pass.

- [ ] **Step 5: Write failing password and atomicity tests**

Add cases for dedicated add/replace, existing saved assignment, new shared
password, remove dedicated, unassign reusable without deleting it,
password-only update with an empty public diff, locked/missing vault rejection,
eligibility blocker rejection, stale config rejection before vault mutation,
vault conflict, and an injected storage failure. Snapshot config bytes, sealed
vault bytes and `PasswordFor` before each rejected call and assert all three are
unchanged afterward.

- [ ] **Step 6: Run password tests and verify RED**

Run:

```bash
go test ./internal/application -run 'TestUpdateConnection.*Password|TestUpdateConnection.*Atomic' -count=1
```

Expected: tests fail because `UpdateConnection` does not yet dispatch password mutations.

- [ ] **Step 7: Implement atomic password updates**

Map update kinds to `secret.PasswordMutation`, including remove. For add or
replace, require the current `PasswordEligibility` report to be storable. For
remove, do not apply eligibility blockers. Require a present unlocked secret
service for every non-unchanged branch.

Call `WithPasswordMutation` and, inside its callback while `saveMutex` is held,
re-resolve and re-plan the config update, append the sealed vault change to the
same `storage.Request`, then call `commitPlannedRequest`. If there are no config
field changes, still locate the Host and compare base/disk, but send only the
vault change. Convert the result to `SaveResult` while omitting vault paths and
diffs from the public preview.

- [ ] **Step 8: Run Task 2 tests and commit**

Run:

```bash
go test ./internal/application ./internal/secret -count=1
go test -race ./internal/application ./internal/secret -count=1
git diff --check
```

Expected: all pass.

Commit:

```bash
git add internal/application/connectionupdate.go \
  internal/application/connectionupdate_test.go internal/application/connectioncreate.go \
  internal/application/edit.go
git commit -m "feat: update connection settings atomically"
```

---

### Task 3: Strict HTTP and browser client boundary

**Files:**
- Modify: `internal/httpserver/connections.go`
- Modify: `internal/httpserver/connections_test.go`
- Modify: `web/src/api/config.ts`
- Modify: `web/src/api/config.test.ts`

**Interfaces:**
- Consumes: generated `api.UpdateConnectionRequest`, application update types, `ConnectionHandlers.Keys`, and `SaveResult`.
- Produces: `PATCH /api/v1/connections` handler and `configApi.updateConnection(request: UpdateConnectionRequest): Promise<SaveResult>`.

- [ ] **Step 1: Write failing HTTP decoding and endpoint tests**

Add request helpers and table tests covering every setting set/inherit branch,
all five password branches, unknown fields, malformed union tags, missing base,
oversized strings, invalid key IDs, no change, complex field, key not found,
vault states, password ineligibility, config conflict and a safe success
response. On success assert the stored secret is usable but neither response
body nor preview contains plaintext or `sshc/secrets.vault`.

- [ ] **Step 2: Run HTTP tests and verify RED**

Run:

```bash
go test ./internal/httpserver -run 'TestUpdateConnectionEndpoint' -count=1
```

Expected: 404 or compilation failure because PATCH is not registered.

- [ ] **Step 3: Implement strict PATCH decoding and problem mappings**

Register `engine.PATCH("/api/v1/connections", handlers.Update)`. Decode the top
level with the existing strict body decoder. Defer wiping the raw password
union buffer. Decode each optional raw setting union and the required password
union with `decodeConnectionAuthentication` so unknown fields and trailing JSON
are rejected. Enforce the OpenAPI size bounds at the HTTP boundary, then call
`Service.UpdateConnection`; request inventory only when IdentityFile has a
`set` action.

Map existing errors plus `ErrNoConnectionUpdate` to 400,
`ErrComplexConnectionField` and invalid key to 422, missing Host/credential to
404, config/vault conflicts to 409, and locked/missing vault through the
existing password problem mapper.

- [ ] **Step 4: Run HTTP tests and verify GREEN**

Run:

```bash
go test ./internal/httpserver -run 'TestUpdateConnectionEndpoint' -count=1
```

Expected: PASS.

- [ ] **Step 5: Write failing browser client tests**

Import generated `UpdateConnectionRequest` in `web/src/api/config.test.ts`,
call the wished-for `configApi.updateConnection`, and assert it sends exactly:

```ts
expect(path).toBe("/api/v1/connections");
expect(init.method).toBe("PATCH");
expect(JSON.parse(String(init.body))).toEqual(request);
```

Return an existing `SaveResult` fixture and assert runtime validation rejects a
malformed response and propagates a typed Problem.

- [ ] **Step 6: Run browser client tests and verify RED**

Run:

```bash
npm test --prefix web -- src/api/config.test.ts
```

Expected: compilation/test failure because the type export and method do not exist.

- [ ] **Step 7: Implement the PATCH client**

Export `UpdateConnectionRequest` and `UpdateConnectionPassword` from generated
components. Generalise `postJSON` to a private `mutateJSON(path, method, body)`
without changing existing POST callers, then add:

```ts
async updateConnection(request: UpdateConnectionRequest): Promise<SaveResult> {
  return validateSaveResult(await mutateJSON<unknown>(
    "/api/v1/connections", "PATCH", request,
  ));
}
```

- [ ] **Step 8: Run Task 3 tests and commit**

Run:

```bash
go test ./internal/httpserver ./internal/api -count=1
npm test --prefix web -- src/api/config.test.ts
npm run typecheck --prefix web
git diff --check
```

Expected: all pass.

Commit:

```bash
git add internal/httpserver/connections.go internal/httpserver/connections_test.go \
  web/src/api/config.ts web/src/api/config.test.ts
git commit -m "feat: expose connection update endpoint"
```

---

### Task 4: Stable Basic connection form

**Files:**
- Create: `web/src/connections/basicFields.ts`
- Create: `web/src/connections/basicFields.test.ts`
- Create: `web/src/connections/ConnectionBasicForm.tsx`
- Create: `web/src/connections/ConnectionBasicForm.test.tsx`
- Modify: `web/src/i18n/messages.ts`

**Interfaces:**
- Consumes: `HostDetail`, `UpdateConnectionRequest`, `SaveResult`, selectable private-key inventory, vault/credential/eligibility APIs, existing form primitives and `PasswordField`.
- Produces: `deriveBasicField(detail, keyword, fallback): BasicFieldState` and `ConnectionBasicForm` with props `{detail, problem, onSave, keys?, secrets?}`.
- `onSave` signature: `(request: UpdateConnectionRequest) => Promise<void>` so the form clears drafts only after its owner finishes reload.

- [ ] **Step 1: Write failing pure field-derivation tests**

Test HostName, User and Port in direct, inherited and default states. A direct
field is one matching field in `detail.form.fields`; inherited uses the first
matching `detail.effective.entries` whose source is not the selected direct
line; defaults are alias for HostName, empty User and `22` Port. Assert duplicate
direct fields return `{editable:false, origin:"complex"}` and include no
automatic update.

The expected shape is:

```ts
type BasicFieldState = {
  keyword: "HostName" | "User" | "Port";
  value: string;
  origin: "direct" | "inherited" | "default" | "complex";
  source?: { path?: string; absolute?: string; line?: number };
  editable: boolean;
};
```

- [ ] **Step 2: Run derivation tests and verify RED**

Run:

```bash
npm test --prefix web -- src/connections/basicFields.test.ts
```

Expected: module-not-found failure.

- [ ] **Step 3: Implement pure field derivation and verify GREEN**

Implement case-insensitive matching, explicit defaults and source labelling.
Do not mutate `HostDetail`. Run the same test and expect PASS.

- [ ] **Step 4: Write failing ConnectionBasicForm interaction tests**

Build injected config/key/secret API fixtures. Cover:

- all three fields render even when `form.fields` is empty;
- opening and saving no changes is disabled and sends nothing;
- editing an inherited value emits `set`, while **Use inherited/default** emits
  `inherit` only for a currently direct value;
- empty User emits `inherit`; invalid host/user/port blocks submission;
- duplicate fields and custom/multiple IdentityFile remain visible read-only;
- one inventoried direct key is selected, another key emits IdentityFile set,
  and agent/inherited emits IdentityFile inherit;
- absent/locked/unlocked vault states do not block config-only edits;
- password text is never prefilled, empty means `unchanged`, replace/assign/new
  shared/remove build the exact password union, and remove requires confirmation;
- eligibility blockers prevent add/replace but not removal;
- selection/detail change and every failed submit clear password/passphrase
  inputs while preserving non-secret drafts on failure;
- one Save button submits all explicit changes in one request.

- [ ] **Step 5: Run form tests and verify RED**

Run:

```bash
npm test --prefix web -- src/connections/ConnectionBasicForm.test.tsx
```

Expected: component-not-found failure.

- [ ] **Step 6: Implement the focused Basic form**

Use two existing visual cards headed Connection and Authentication. Keep direct
draft values separate from their original derived states; build optional
hostName/user/port/identityFile operations only when a field changed or the
explicit inherit action is active. Always include `password:{kind:"unchanged"}`
when password controls are untouched.

Load `keys.inventory()` and `passwordVault()` together. Load credentials only
when unlocked and filter `kind === "password"`. Load eligibility for the alias.
Use existing initialise/unlock APIs inline. Never store master password or
account-password text in a durable draft type. Clear secrets in cleanup,
identity-change effects, success, and catch paths.

Map current direct IdentityFile value to a private key by canonical displayed
path `~/.ssh/${relativePath}`. Unknown custom paths and multiple direct values
are read-only with an Advanced hint. Do not claim inherited cumulative keys are
removed when the direct field is inherited.

Add Japanese and English messages for headings, origin/source labels,
validation, vault states, password operations, explicit removal confirmation,
complex/custom key hints, Save Basic settings and failure states. Do not reuse
creation copy when it describes mutually exclusive authentication.

- [ ] **Step 7: Run Task 4 tests and commit**

Run:

```bash
npm test --prefix web -- src/connections/basicFields.test.ts src/connections/ConnectionBasicForm.test.tsx
npm run typecheck --prefix web
git diff --check
```

Expected: all pass.

Commit:

```bash
git add web/src/connections/basicFields.ts web/src/connections/basicFields.test.ts \
  web/src/connections/ConnectionBasicForm.tsx web/src/connections/ConnectionBasicForm.test.tsx \
  web/src/i18n/messages.ts
git commit -m "feat: add editable connection basic form"
```

---

### Task 5: Detail-page integration, end-to-end coverage and release verification

**Files:**
- Modify: `web/src/connections/HostDetail.tsx`
- Modify: `web/src/connections/HostDetail.test.tsx`
- Modify: `web/src/connections/ConnectionsPage.tsx`
- Modify: `web/src/connections/ConnectionsPage.test.tsx`
- Modify: `web/e2e/connections.spec.ts`
- Modify: `web/e2e/password.spec.ts`
- Modify: `web/e2e/secrets.spec.ts`
- Generated: `internal/ui/dist/index.html`
- Generated: `internal/ui/dist/assets/*`

**Interfaces:**
- Consumes: `ConnectionBasicForm`, `configApi.updateConnection`, existing selection/reload/preview flow.
- Produces: integrated Basic tab with password management removed from Diagnostics and persisted updates visible after reload.

- [ ] **Step 1: Write failing HostDetail and page integration tests**

Update `HostDetail.test.tsx` so Basic expects stable labels rather than the raw
category field list. Assert Advanced still contains non-form basic directives
such as `IdentitiesOnly`, Jump/Raw/Effective remain unchanged, Diagnostics still
runs checks, and no Stored password panel is duplicated there.

In `ConnectionsPage.test.tsx`, inject or spy on `configApi.updateConnection`,
submit a Basic request, and assert success reloads overview/detail, preserves
selection and shows returned preview. Assert a rejected request stays selected
and exposes its Problem without issuing a separate config save or password API
write.

- [ ] **Step 2: Run integration tests and verify RED**

Run:

```bash
npm test --prefix web -- src/connections/HostDetail.test.tsx src/connections/ConnectionsPage.test.tsx
```

Expected: failures because HostDetail still owns directive Basic and password is in Diagnostics.

- [ ] **Step 3: Integrate the Basic form**

Add `onBasicSave(request): Promise<void>` to `HostDetailPanel`. Render
`ConnectionBasicForm` for the Basic tab. Change the old directive renderer to
Jump/Advanced only and route stable form keywords (`hostname`, `user`, `port`,
`identityfile`) out of Advanced while keeping other basic-category directives
there. Remove `PasswordPanel` import/rendering from Diagnostics.

In `ConnectionsPage`, call `configApi.updateConnection`, set its preview,
reload overview, then reload the current Host detail and keep the same identity.
On failure set the existing Problem and rethrow so the form can clear secret
inputs without clearing non-secret drafts. Do not call the legacy field-save or
password endpoints for one Basic submission.

- [ ] **Step 4: Run integration tests and verify GREEN**

Run the Step 2 command again. Expected: PASS.

- [ ] **Step 5: Add one real end-to-end edit journey**

Extend the isolated SSH-home journey in `web/e2e/connections.spec.ts`. Append a
sparse `Host edge` block to the fixture config before opening the application.
Open its detail and assert Host name,
User and Port are visible with defaults. Set Host name, User and Port, select an
inventoried key, save, reload, and assert the exact Host block contains each
directive once. Use the fixture's already initialised and unlocked vault, store
a dedicated password, assert no password appears in visible page text or the
config file, then remove it through the explicit confirmation and verify the
status after reload. Update `web/e2e/password.spec.ts` and
`web/e2e/secrets.spec.ts` to operate from Basic without clicking Diagnostics.
Reuse existing fixtures; do not introduce an external service.

- [ ] **Step 6: Run focused frontend and E2E verification**

Run:

```bash
npm test --prefix web -- src/connections
npm run typecheck --prefix web
npm run e2e --prefix web -- --grep "edits connection basics"
```

Expected: all focused tests pass; platform-specific existing skips remain skips.

- [ ] **Step 7: Run full verification and inspect the shipped diff**

Run fresh, in this order:

```bash
make verify-generated
make test
make build
make e2e
git diff --check
git status --short
git diff --stat df02f0e..HEAD
```

Then verify no dependency manifest changed:

```bash
git diff --name-only df02f0e..HEAD -- go.mod go.sum package.json package-lock.json web/package.json web/package-lock.json
```

Expected: generated check, all Go and race tests, all Vitest tests, TypeScript,
build and Playwright pass; only documented platform skips remain; the manifest
command prints nothing.

- [ ] **Step 8: Commit source and rebuilt embedded UI assets**

Stage only the Task 5 source/tests and the tracked output produced by
`make build`. Commit:

```bash
git add web/src/connections/HostDetail.tsx web/src/connections/HostDetail.test.tsx \
  web/src/connections/ConnectionsPage.tsx web/src/connections/ConnectionsPage.test.tsx \
  web/e2e/connections.spec.ts web/e2e/password.spec.ts web/e2e/secrets.spec.ts \
  internal/ui/dist
git commit -m "feat: integrate editable connection basics"
```

- [ ] **Step 9: Review, push and verify remote CI**

Request a whole-branch code review against `df02f0e`, fix every Critical or
Important finding with a regression test, rerun the focused command from the
task that exposed the issue and all Step 7 commands, then push the current branch:

```bash
git push origin HEAD
```

Use `gh run list --branch "$(git branch --show-current)" --limit 3` and
`gh run watch <run-id> --exit-status` to verify the pushed commit's CI. Report
the exact commit, branch, local verification counts, remote job result, and any
skips or unverified boundary.
