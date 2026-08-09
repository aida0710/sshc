# Connection Creation Modal Implementation Plan

**Status:** approved for implementation.

**Goal:** Replace the alias-only connection creator with one modal that creates a usable SSH connection immediately, supports explicit password or key authentication, shows every declared save group (including empty nested groups), and opens the new connection's Basic detail after creation.

**Architecture:** A dedicated application service validates the complete creation request and builds one transaction containing the SSH config change and, for password modes, the encrypted vault change. The secret service exposes a callback-scoped mutation that edits a cloned vault and publishes it in memory only after the shared storage commit succeeds. The HTTP endpoint is described in OpenAPI, and the React modal uses generated types plus the existing vault and key APIs.

**Tech Stack:** Existing Go, Echo, OpenAPI/oapi-codegen, React, TypeScript, Vitest, and Playwright dependencies only. No package or module additions.

## Global constraints

- Blank Port is accepted but saved as an explicit `Port 22`.
- Optional User is omitted when blank.
- A group destination is valid when declared by the Include graph even if it contains no files.
- Dedicated passwords are encrypted and bound only to the new connection; they never appear in reusable credential lists.
- Config and encrypted vault bytes commit atomically.
- API responses and previews never contain plaintext secrets or credential-bearing sealed bytes.
- Creation does not launch Terminal.
- On success the modal closes, the tree reloads, and the new connection opens on Basic.
- Existing user changes and existing password records remain intact.
- No dependency installation or generated dependency drift.

### Task 1: Make vault mutation rollback-safe and add dedicated passwords

**Files:**
- Modify: `internal/secret/vault.go`
- Modify: `internal/secret/vault_test.go`
- Modify: `internal/secret/service.go`
- Modify: `internal/secret/service_test.go`

1. Add failing tests for dedicated-password set, lookup, removal, rename behavior, exclusion from reusable credentials, and failed-commit rollback.
2. Extend the encrypted document with an optional `DedicatedPasswords` collection while keeping schema version 2 backward compatible.
3. Add in-memory dedicated password operations. `SecretFor(alias)` checks dedicated storage first; assigning a reusable credential removes a dedicated secret for that alias.
4. Add a typed password mutation:
   - `dedicated_password` with plaintext password
   - `saved_password` with credential name
   - `new_shared_password` with credential name and plaintext password
5. Implement `WithPasswordMutation(mutation, commitCallback)`:
   - hold the secret mutex;
   - clone the live vault;
   - mutate only the clone;
   - seal the clone and create a preconditioned vault storage change;
   - call the callback with that change;
   - publish the clone and new baseline only when the callback succeeds.
6. Run the focused secret tests and commit.

### Task 2: Add the atomic application creation service

**Files:**
- Create: `internal/application/connectioncreate.go`
- Create: `internal/application/connectioncreate_test.go`
- Modify: `internal/application/service.go` as needed

1. Add table-driven failing tests for:
   - required alias and HostName;
   - alias grammar and reachable-alias uniqueness;
   - optional User;
   - blank Port becoming 22 and invalid port rejection;
   - declared empty nested group acceptance;
   - unknown group and existing destination file rejection;
   - private-key selection and non-private-key rejection;
   - all three password modes;
   - config-only key creation;
   - atomic config-plus-vault commit;
   - conflict/failure leaving both disk and in-memory vault unchanged;
   - secret-free preview and result.
2. Define `CreateConnectionRequest`, `CreateAuthentication`, and `CreateConnectionResult`.
3. Build directives with the existing `buildLine`/`config.RenderArgument` path:
   - `Host <alias>`
   - `HostName <host>`
   - optional `User <user>`
   - always `Port <normalized port>`
   - `IdentityFile ~/.ssh/<relative key path>` only for key authentication.
4. Resolve destination:
   - no group: append the block to the entry config;
   - group: create `connections/<group>/<alias>.conf`, requiring the group to be declared and the file to be absent.
5. For password modes, invoke `WithPasswordMutation` and append its vault change to the exact same `storage.Request` as the config change. For key mode, commit only config.
6. Preserve lock order: secret mutex, application save mutex, then storage manager commit.
7. Run focused application and secret tests and commit.

### Task 3: Expose the creation endpoint through OpenAPI

**Files:**
- Modify: `api/openapi.yaml`
- Regenerate: `internal/api/models.gen.go`
- Regenerate: `internal/api/server.gen.go`
- Regenerate: `web/src/api/schema.d.ts`
- Create: `internal/httpserver/connections.go`
- Create: `internal/httpserver/connections_test.go`
- Modify: `internal/httpserver/server.go`
- Modify: relevant contract/security tests

1. Add a failing HTTP test for `POST /api/v1/connections` covering 201, validation errors, locked/uninitialized vault, conflict, and secret-free output.
2. Define request authentication as a discriminated union:
   - `{kind:"dedicated_password", password}`
   - `{kind:"saved_password", credential}`
   - `{kind:"new_shared_password", credential, password}`
   - `{kind:"identity_file", keyId}`
3. Define the response with transaction ID, identity, and safe config preview.
4. Map application errors to stable problem codes and register the route under the existing authenticated API.
5. Run generation and verify no dependency files change.
6. Run focused HTTP/contract/security tests and commit.

### Task 4: Add typed web API functions

**Files:**
- Modify: `web/src/api/config.ts`
- Modify: `web/src/api/config.test.ts`
- Modify: `web/src/api/keys.ts`
- Modify: `web/src/api/keys.test.ts`

1. Add failing tests for request/response typing, 201 handling, problem propagation, and private-key filtering.
2. Add `createConnection` using generated OpenAPI types.
3. Add a key inventory helper that returns only selectable private identities.
4. Run focused Vitest tests and TypeScript typecheck, then commit.

### Task 5: Build the creation modal

**Files:**
- Create: `web/src/connections/CreateConnectionModal.tsx`
- Create: `web/src/connections/CreateConnectionModal.test.tsx`
- Modify: `web/src/i18n/messages.ts`
- Modify: related CSS/component utilities only when required

1. Add failing component tests for:
   - alias, destination, HostName, optional User, and optional Port controls;
   - blank Port submission;
   - all declared groups including empty `home-lab/others`;
   - dedicated, existing shared, new shared, and key authentication branches;
   - vault initialize/unlock flow;
   - private keys only;
   - inline field and server errors;
   - secret field clearing on cancel, Escape, failure, and success;
   - dialog semantics and initial focus.
2. Implement a two-section modal: Connection and Authentication.
3. Treat dedicated password as the default password path and label it as connection-only encrypted storage.
4. Disable submission while required data is unavailable or invalid; do not retain secrets in surrounding page state.
5. Run the component tests and typecheck, then commit.

### Task 6: Integrate creation into Connections and select the result

**Files:**
- Modify: `web/src/connections/ConnectionsPage.tsx`
- Modify: `web/src/connections/ConnectionsPage.test.tsx`
- Modify: `web/src/connections/HostDetail.tsx`
- Modify: `web/src/connections/HostDetail.test.tsx`
- Modify: `web/src/connections/blocks.ts`
- Modify: `web/src/connections/blocks.test.ts`

1. Add failing tests for opening the modal, successful refresh and selection, Basic-tab reset, and no terminal launch.
2. Remove alias-only creation state, target-file selection derived only from existing files, and `appendHostBlock`.
3. Feed modal destinations from `overview.groups` so declared empty nested groups are displayed.
4. On success: apply safe preview if needed, close modal, reload overview/tree, select the response identity, and open Host detail.
5. Reset Host detail to Basic whenever the selected identity changes.
6. Preserve duplicate/delete helpers and their tests.
7. Run focused Vitest and typecheck, then commit.

### Task 7: End-to-end and regression verification

**Files:**
- Modify/Create: relevant `web/e2e` connection creation specs
- Modify: fixtures only as required

1. Add an end-to-end case that selects an empty nested group and creates a key-authenticated connection.
2. Add an end-to-end case for a dedicated encrypted password, including vault setup/unlock where the fixture requires it.
3. Assert the created host immediately appears selected, its Basic detail is visible, `Port 22` is present, and no Terminal action occurs.
4. Run:
   - `make verify-generated`
   - focused Go tests
   - focused Vitest tests
   - `npm run typecheck` via the existing project setup
   - `make build`
   - focused Playwright connection specs
   - `make test`
5. Inspect `git diff --check`, `git status --short`, and dependency manifests. Document anything not run or environment-blocked.
6. Invoke verification-before-completion and only then report completion.


