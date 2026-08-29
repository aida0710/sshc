# Info and Headless Sync CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an engine-independent `sshc info <alias>` and an authenticated headless sync CLI which delegates every sync decision and mutation to the running engine.

**Architecture:** `info` exposes an allowlisted description of the exact `sshclient.Target` used by CLI connections. Sync commands obtain a command-scoped Web API session through a handoff-authenticated bridge, then call the existing `/api/v1/sync` and `/api/v1/actions` routes with the generated API models; the CLI owns only prompting, orchestration, and rendering.

**Tech Stack:** Go 1.26, Echo v5, existing `internal/api` OpenAPI models, `golang.org/x/term`, Go `net/http`, existing application/effective/sshclient/remotesync/session packages.

**Spec:** `docs/superpowers/specs/2026-08-29-info-and-headless-sync-cli-design.md`

## Global Constraints

- Do not add or install a local package; use only the repository's pinned Go and Node dependencies.
- Preserve the existing untracked `desktop/` directory and `sshc` file.
- Do not add `sshc connect`; keep `sshc ssh` as the human TUI/explicit connection namespace.
- `info` must use the same application resolver and `sshclient.NewTarget` path as `Open` and `Run`; never use `effective.Project` for connection values.
- Every sync command requires a compatible running engine and an unlocked vault; do not start an engine implicitly.
- Do not implement S3, snapshot, encryption, CAS, pull transaction, auto-sync, or force semantics in the CLI.
- Never accept sync credentials or keys through argv, environment variables, or a file.
- Never print password, passphrase, access key ID, secret access key, sync key except the one-time generated setup key, session cookie, CSRF token, action token, handoff secret, `SetEnv` values, or `ProxyCommand` text.
- `--force` is the confirmation; do not add a second yes/no prompt, unconditional PUT, automatic re-preview, or automatic retry.
- Exit codes are success `0`, operational failure `1`, usage `2`, and interrupt `130`.
- Follow TDD for every behavior change: add one failing test, observe the expected failure, add the minimum implementation, and rerun the focused test before refactoring.
- Use `apply_patch` for source and documentation edits.

---

## File Map

### CLI grammar and dispatch

- Modify `cmd/sshc/invocation.go`: add typed info/sync invocations and usage text.
- Modify `cmd/sshc/invocation_test.go`: lock the accepted and rejected argv matrix.
- Modify `cmd/sshc/main.go`: dispatch info and sync with signal cancellation.

### Shared SSH target and info presentation

- Modify `internal/app/ssh.go`: expose the already-built connection target through `CLIConnection.Resolve`.
- Modify `internal/app/ssh_test.go`: prove `Resolve`, `Open`, and `Run` use the same target builder.
- Create `cmd/sshc/info.go`: safe DTO, human renderer, JSON renderer, and command runner.
- Create `cmd/sshc/info_test.go`: resolver parity, secret-bearing directive redaction, output, and failure tests.

### Command-scoped API authentication

- Modify `internal/session/manager.go`: issue, expire, look up, and revoke command-scoped sessions.
- Modify `internal/session/manager_test.go`: lifetime and browser-bootstrap isolation tests.
- Modify `internal/session/action.go`: make action issuance/consumption reject expired sessions through the shared lookup.
- Modify `internal/session/action_test.go`: expired command session cannot issue or consume actions.
- Create `internal/httpserver/cli_session.go`: handoff-authenticated API-session issue/revoke routes.
- Create `internal/httpserver/cli_session_test.go`: authentication, cookie, CSRF, TTL, and bootstrap-isolation tests.
- Modify `internal/httpserver/connect.go`: register the new CLI session routes and reuse the session-cookie writer.
- Modify `internal/httpserver/handlers.go`: share the existing strict session cookie construction.
- Modify `internal/httpserver/server_test.go`: include the new routes in route inventory.

### CLI engine API transport

- Create `cmd/sshc/engine_api.go`: authenticated API session, bounded requests/responses, typed problem errors, revoke, and JSON envelopes.
- Create `cmd/sshc/engine_api_test.go`: exact-origin headers, cookie/CSRF, redirect refusal, size bounds, cancellation, revoke, and error redaction.
- Modify `cmd/sshc/status.go`: reuse the shared handoff request primitive without changing `sshc status` output.
- Modify `cmd/sshc/vault.go`: reuse the shared one-shot secret body primitive while preserving vault uncertainty behavior and zeroing.
- Modify `cmd/sshc/status_test.go` and `cmd/sshc/vault_test.go`: regression coverage for the refactor.

### Sync commands

- Create `cmd/sshc/sync.go`: status, push, pull, now, auto orchestration and output.
- Create `cmd/sshc/sync_test.go`: fake-engine API behavior, force semantics, JSON contract, error mapping, and exit codes.
- Create `cmd/sshc/sync_setup.go`: TTY wizard and zeroable setup payload encoding.
- Create `cmd/sshc/sync_setup_test.go`: prompting, no-echo secrets, check-before-save, existing/empty/incomplete targets, cancellation, and redaction.
- Modify `cmd/sshc/vault.go`: extract reusable secret prompting/JSON string helpers without weakening current tests.

### Documentation and verification

- Modify `README.md`: document `info` and sync commands.
- Modify `docs/headless-examples.md`: add setup and unattended engine examples without putting secrets in unit files.
- Modify `docs/manual-acceptance.md`: add manual info/setup/force safety checks.
- Modify `scripts/ci/cli-smoke.sh` and `scripts/ci/cli-smoke.ps1`: exercise `info` on the release binary.

---

### Task 1: Typed CLI Grammar

**Files:**
- Modify: `cmd/sshc/invocation.go`
- Modify: `cmd/sshc/invocation_test.go`

**Interfaces:**
- Produces: `invocationInfo`, `invocationSync`, `syncInvocation`, and `parseSyncInvocation([]string) (invocation, error)`.
- Consumes: existing `invocation.JSON`, `copyInvocationArgs`, and usage-error conventions.

- [ ] **Step 1: Write failing parser tests**

Add table-driven tests which require these exact forms:

```go
tests := []struct {
	argv   []string
	kind   invocationKind
	alias  string
	action syncAction
	force  bool
	json   bool
	enable bool
}{
	{argv: []string{"sshc", "info", "edge"}, kind: invocationInfo, alias: "edge"},
	{argv: []string{"sshc", "info", "edge", "--json"}, kind: invocationInfo, alias: "edge", json: true},
	{argv: []string{"sshc", "sync"}, kind: invocationSync, action: syncStatus},
	{argv: []string{"sshc", "sync", "--json"}, kind: invocationSync, action: syncStatus, json: true},
	{argv: []string{"sshc", "sync", "setup"}, kind: invocationSync, action: syncSetup},
	{argv: []string{"sshc", "sync", "push", "--force", "--json"}, kind: invocationSync, action: syncPush, force: true, json: true},
	{argv: []string{"sshc", "sync", "pull", "--force"}, kind: invocationSync, action: syncPull, force: true},
	{argv: []string{"sshc", "sync", "now", "--json"}, kind: invocationSync, action: syncNow, json: true},
	{argv: []string{"sshc", "sync", "auto", "on"}, kind: invocationSync, action: syncAuto, enable: true},
	{argv: []string{"sshc", "sync", "auto", "off", "--json"}, kind: invocationSync, action: syncAuto, json: true},
}
```

Reject missing info aliases, `info --json edge`, setup JSON/force flags, force on now/auto, duplicate flags, unknown actions, missing auto state, and extra positional arguments. Require usage to name every new form and continue rejecting `sshc connect`.

- [ ] **Step 2: Run the parser tests and verify RED**

Run: `go test ./cmd/sshc -run 'TestParseInfoAndSyncInvocations|TestInfoAndSyncRejectAmbiguousArguments|TestUsageNamesInfoAndSync' -count=1`

Expected: FAIL because the invocation kinds, sync types, and parser branches do not exist.

- [ ] **Step 3: Add the typed parser**

Implement these types without passing raw sync argv beyond parsing:

```go
type syncAction uint8

const (
	syncInvalid syncAction = iota
	syncStatus
	syncSetup
	syncPush
	syncPull
	syncNow
	syncAuto
)

type syncInvocation struct {
	Action  syncAction
	Force   bool
	JSON    bool
	Enabled bool
}

type invocation struct {
	Kind      invocationKind
	Args      []string
	Port      int
	Replace   bool
	JSON      bool
	Transport *transportInvocation
	Sync      *syncInvocation
}
```

Parse `info` as one alias followed by optional `--json`. Parse sync flags once, reject duplicates, and validate flags against the selected action. Update usage with the spec's exact command forms.

- [ ] **Step 4: Run focused and complete parser tests**

Run: `go test ./cmd/sshc -run 'Test.*Invocation|TestUsage' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the grammar**

```bash
git add cmd/sshc/invocation.go cmd/sshc/invocation_test.go
git commit -m "feat(cli): reserve info and sync commands"
```

### Task 2: Shared SSH Target and `sshc info`

**Files:**
- Modify: `internal/app/ssh.go`
- Create: `internal/app/ssh_test.go`
- Create: `cmd/sshc/info.go`
- Create: `cmd/sshc/info_test.go`
- Modify: `cmd/sshc/main.go`

**Interfaces:**
- Produces: `func (c CLIConnection) Resolve(alias string) (sshclient.Target, error)`.
- Produces: `func runInfo(alias, home string, asJSON bool, stdout, stderr io.Writer) int`.
- Consumes: Task 1 `invocationInfo` and existing `app.NewCLIConnection`.

- [ ] **Step 1: Write the failing target-sharing test**

Construct a temporary home with `Include`, `Match user`, `ProxyJump`, `IdentityFile`, and sshc encoding metadata. Require the public method to return the same `sshclient.Target` fields which `CLIConnection.Open`/`Run` obtain from `c.parts.target`:

```go
target, err := connection.Resolve("edge")
if err != nil {
	t.Fatal(err)
}
if target.HostName != "edge.internal" || target.Port != "22" || target.User != "operator" {
	t.Fatalf("target = %+v", target)
}
if len(target.Jump) != 1 || target.Jump[0].Alias != "bastion" {
	t.Fatalf("jump = %+v", target.Jump)
}
```

- [ ] **Step 2: Verify the target test fails**

Run: `go test ./internal/app -run TestCLIConnectionResolveUsesTheConnectionTarget -count=1`

Expected: FAIL because `CLIConnection.Resolve` does not exist.

- [ ] **Step 3: Expose the existing target builder**

Add only this delegation; do not create another resolver:

```go
func (c CLIConnection) Resolve(alias string) (sshclient.Target, error) {
	return c.parts.target(alias)
}
```

Run: `go test ./internal/app -run TestCLIConnectionResolveUsesTheConnectionTarget -count=1`

Expected: PASS.

- [ ] **Step 4: Write failing info output and redaction tests**

Require one allowlisted DTO recursively mapped from `sshclient.Target`:

```go
type infoDocument struct {
	SchemaVersion              int               `json:"schemaVersion"`
	Alias                      string            `json:"alias"`
	Destination                infoDestination   `json:"destination"`
	IdentityFiles              []string          `json:"identityFiles"`
	IdentitiesOnly             bool              `json:"identitiesOnly"`
	ProxyJump                  []infoHop         `json:"proxyJump"`
	ProxyCommandConfigured     bool              `json:"proxyCommandConfigured"`
	Encoding                   string            `json:"encoding"`
	AuthenticationMethods      []string          `json:"authenticationMethods"`
	RequestTTY                 string            `json:"requestTTY"`
	StrictHostKeyChecking      string            `json:"strictHostKeyChecking"`
	ConnectTimeoutSeconds      int64             `json:"connectTimeoutSeconds"`
	ServerAliveIntervalSeconds int64             `json:"serverAliveIntervalSeconds"`
	ServerAliveCountMax        int               `json:"serverAliveCountMax"`
	AgentForward               bool              `json:"agentForward"`
	Notices                    []infoNotice       `json:"notices"`
}
```

Tests must assert `Port 22`, Match-derived values, jump order, absolute identities, encoding, and non-nil empty arrays. Put sentinel secrets in `SetEnv` and `ProxyCommand`, run both renderers, and assert neither sentinel appears. Assert `runInfo` succeeds without a handoff file or engine.

- [ ] **Step 5: Verify info tests fail**

Run: `go test ./cmd/sshc -run 'TestInfo' -count=1`

Expected: FAIL because `runInfo` and the DTO/renderers do not exist.

- [ ] **Step 6: Implement the allowlisted info command**

Create the connection with nil credential callbacks, validate the alias with `validate.Alias`, call `Resolve`, map only the specified fields, and render either aligned human rows or a single JSON object. Map resolver refusal and invalid alias to exit 1 and 2 respectively. Ensure slices are initialized with `make(..., 0, n)`.

Dispatch `invocationInfo` in `main.go`:

```go
case invocationInfo:
	return runInfo(called.Args[0], home, called.JSON, os.Stdout, os.Stderr)
```

- [ ] **Step 7: Run info and regression tests**

Run: `go test ./internal/app ./cmd/sshc -run 'TestCLIConnectionResolve|TestInfo|TestParseInfo' -count=1`

Expected: PASS.

- [ ] **Step 8: Commit info**

```bash
git add internal/app/ssh.go internal/app/ssh_test.go cmd/sshc/info.go cmd/sshc/info_test.go cmd/sshc/main.go
git commit -m "feat(cli): describe resolved SSH targets"
```

### Task 3: Expiring Command Sessions

**Files:**
- Modify: `internal/session/manager.go`
- Modify: `internal/session/manager_test.go`
- Modify: `internal/session/action.go`
- Modify: `internal/session/action_test.go`

**Interfaces:**
- Produces: `func (m *Manager) IssueExpiring(lifetime time.Duration) (Credentials, error)`.
- Produces: `func (m *Manager) Revoke(sessionID string) bool`.
- Produces: a shared locked lookup which removes expired sessions.
- Consumes: existing `Manager.Now`, `Credentials`, CSRF verification, and action-token storage.

- [ ] **Step 1: Write failing expiry and revoke tests**

Use a controllable `manager.Now` and require this lifecycle:

```go
credentials, err := manager.IssueExpiring(10 * time.Minute)
if err != nil {
	t.Fatal(err)
}
if !manager.Authenticate(credentials.SessionID) || !manager.VerifyCSRF(credentials.SessionID, credentials.CSRFToken) {
	t.Fatal("issued command session is not usable")
}
now = now.Add(10 * time.Minute)
if manager.Authenticate(credentials.SessionID) {
	t.Fatal("expired command session survived")
}
```

Also require `Revoke` to return true once/false twice, reject non-positive lifetime, leave browser bootstrap sessions unexpired, and leave an already-issued browser bootstrap token consumable.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/session -run 'TestExpiringSession|TestRevokingSession|TestCommandSessionDoesNotConsumeBrowserBootstrap' -count=1`

Expected: FAIL because issue/revoke and expiry metadata do not exist.

- [ ] **Step 3: Implement one session issuance primitive**

Add an expiry to `Session`, and factor token generation into one locked helper used by both bootstrap and command issuance:

```go
type Session struct {
	csrfHashes [][sha256.Size]byte
	actions    map[[sha256.Size]byte]actionRecord
	expiresAt  time.Time
}

func (m *Manager) IssueExpiring(lifetime time.Duration) (Credentials, error)
func (m *Manager) Revoke(sessionID string) bool
func (m *Manager) sessionLocked(sessionID string) (Session, bool)
```

Zero `expiresAt` means the existing browser lifetime. A non-zero expiry is invalid at or after its time. Change `Authenticate`, `VerifyCSRF`, and `RenewCSRF` to use the locked lookup so expiry is enforced and removed atomically.

- [ ] **Step 4: Run manager tests GREEN**

Run: `go test ./internal/session -run 'TestBootstrap|TestRenew|TestReissue|TestExpiringSession|TestRevokingSession|TestCommandSession' -count=1`

Expected: PASS.

- [ ] **Step 5: Write failing action-expiry tests**

Require an expired command session to return `ErrUnknownSession` from both `IssueAction` and `ConsumeAction`, and require a browser session to retain current action behavior.

- [ ] **Step 6: Route action lookups through expiry enforcement**

Use `sessionLocked` from `IssueAction` and `ConsumeAction` while already holding `m.mu`. Do not change `ActionTokenTTL`, evidence matching, token burning, or maximum-action behavior.

- [ ] **Step 7: Run the session package**

Run: `go test ./internal/session -count=1`

Expected: PASS.

- [ ] **Step 8: Commit session lifetime support**

```bash
git add internal/session/manager.go internal/session/manager_test.go internal/session/action.go internal/session/action_test.go
git commit -m "feat(session): add expiring CLI sessions"
```

### Task 4: Handoff-Authenticated API Session Bridge

**Files:**
- Create: `internal/httpserver/cli_session.go`
- Create: `internal/httpserver/cli_session_test.go`
- Modify: `internal/httpserver/connect.go`
- Modify: `internal/httpserver/handlers.go`
- Modify: `internal/httpserver/server_test.go`

**Interfaces:**
- Produces: `CLISessionPath = "/cli/session"` and `CLISessionTTL = 10 * time.Minute`.
- Produces: `ConnectHandlers.CLISession` and `ConnectHandlers.RevokeCLISession`.
- Consumes: Task 3 `IssueExpiring`, `Revoke`, existing `cliAuthorised`, `SessionCookie`, and `api.BootstrapResponse`.

- [ ] **Step 1: Write failing route tests**

Test `POST /cli/session` with no/wrong handoff secret returns the same refusal, and a valid secret returns:

```go
if cookie.Name != SessionCookie || !cookie.HttpOnly || cookie.Path != "/" || cookie.SameSite != http.SameSiteStrictMode {
	t.Fatalf("cookie = %+v", cookie)
}
var answer api.BootstrapResponse
json.Unmarshal(response.Body.Bytes(), &answer)
if !manager.VerifyCSRF(cookie.Value, answer.CsrfToken) {
	t.Fatal("CLI CSRF token was not registered")
}
```

Use the returned cookie/CSRF through the real `Security` middleware on a protected test API route. Test `DELETE /cli/session` revokes it. Advance the clock past `CLISessionTTL` and require rejection. Record a Web bootstrap token before CLI issuance and prove it remains consumable afterward.

- [ ] **Step 2: Verify route tests fail**

Run: `go test ./internal/httpserver -run 'TestCLISession' -count=1`

Expected: FAIL because the route and constants do not exist.

- [ ] **Step 3: Factor strict session cookie writing**

Move the existing cookie construction in `Handlers.Bootstrap` to:

```go
func setSessionCookie(c *echo.Context, sessionID string) {
	c.SetCookie(&http.Cookie{
		Name: SessionCookie, Value: sessionID, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
}
```

Keep the Web bootstrap response and security behavior unchanged.

- [ ] **Step 4: Implement issue and revoke routes**

Authenticate both routes with `X-SSHC-CLI`. Issue an expiring normal API session, set the shared cookie, and return only `api.BootstrapResponse{CsrfToken: ...}`. Revoke only the session named by the cookie and return 204; never return either token in an error.

Register the two methods from `registerConnectRoutes`. Add both routes to route-inventory expectations.

- [ ] **Step 5: Run HTTP security regressions**

Run: `go test ./internal/httpserver -run 'TestCLISession|TestBootstrapHandler|TestServerReportsEveryRegisteredRoute|TestSecurity' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit the bridge**

```bash
git add internal/httpserver/cli_session.go internal/httpserver/cli_session_test.go internal/httpserver/connect.go internal/httpserver/handlers.go internal/httpserver/server_test.go
git commit -m "feat(engine): bridge CLI auth to API sessions"
```

### Task 5: Shared Authenticated Engine API Client

**Files:**
- Create: `cmd/sshc/engine_api.go`
- Create: `cmd/sshc/engine_api_test.go`
- Modify: `cmd/sshc/status.go`
- Modify: `cmd/sshc/status_test.go`
- Modify: `cmd/sshc/vault.go`
- Modify: `cmd/sshc/vault_test.go`

**Interfaces:**
- Produces: `openEngineAPI(ctx, stateDir string, base *http.Client) (*engineAPI, error)`.
- Produces: typed `getJSON`, `sendJSON`, `sendSecretJSON`, `issueAction`, and `Close` methods.
- Produces: `engineProblem{Status int, Code string, Retryable bool, OutcomeUnknown bool}`.
- Consumes: Task 4 session route; existing `readHandoff`, `requestStatus`, one-shot vault payload, and generated `api.Problem`.

- [ ] **Step 1: Write failing authentication/client tests**

Create an `httptest.Server` which implements `/cli/status`, `/cli/session`, `/api/v1/sync`, and session DELETE. Require this request sequence and headers:

```text
GET    /cli/status       X-SSHC-CLI
POST   /cli/session      X-SSHC-CLI
GET    /api/v1/sync      Cookie, X-SSHC-CSRF, Sec-Fetch-Site: same-origin
DELETE /cli/session      X-SSHC-CLI, Cookie
```

For mutations additionally require `Origin` equal to the exact handoff origin. Test mismatched owner/version/protocol, locked or missing vault, redirect refusal, over-limit response, malformed/unknown JSON, context cancellation, and `application/problem+json` decoding. Include sentinel token values and assert no returned error contains them.

- [ ] **Step 2: Verify RED**

Run: `go test ./cmd/sshc -run 'TestEngineAPI' -count=1`

Expected: FAIL because `engineAPI` does not exist.

- [ ] **Step 3: Implement the bounded client**

Use these fixed limits:

```go
const (
	maxEngineAPIResponse = 2 << 20
	engineAPITimeout     = 30 * time.Minute
	engineCloseTimeout   = 5 * time.Second
)
```

The open sequence reads one handoff, asks that exact engine for status, validates owner/version/protocol against the handoff, requires an existing unlocked vault, and obtains a CLI session. Clone the caller's client, disable redirects, and set the command timeout without modifying the caller's value.

Each API method adds the cookie, CSRF, and same-origin headers itself. Decode with a bounded reader, reject trailing JSON, and convert non-2xx problem bodies to `engineProblem`. A network error after a mutation request begins sets `OutcomeUnknown`; it must never be silently retried.

- [ ] **Step 4: Run engine-client tests GREEN**

Run: `go test ./cmd/sshc -run 'TestEngineAPI' -count=1`

Expected: PASS.

- [ ] **Step 5: Refactor existing safe primitives under green tests**

Rename `oneShotVaultPayload` to the neutral private `oneShotSecretPayload` and reuse it for both vault and sync secret bodies. Factor handoff-authenticated request creation/redirect refusal where it reduces exact duplication, but preserve:

- vault's 4 KiB body bound;
- byte zeroing on every exit;
- no `GetBody`/redirect replay;
- `change-password` outcome-uncertain message;
- current `sshc status` human and JSON wire shapes.

- [ ] **Step 6: Run existing status/vault suites**

Run: `go test ./cmd/sshc -run 'TestEngineStatus|TestRunStatus|TestStatus|TestRunVault|TestVault|TestOneShot' -count=1`

Expected: PASS with unchanged public output.

- [ ] **Step 7: Commit the common client**

```bash
git add cmd/sshc/engine_api.go cmd/sshc/engine_api_test.go cmd/sshc/status.go cmd/sshc/status_test.go cmd/sshc/vault.go cmd/sshc/vault_test.go
git commit -m "feat(cli): add authenticated engine API client"
```

### Task 6: Sync Status and Stable Result Envelopes

**Files:**
- Create: `cmd/sshc/sync.go`
- Create: `cmd/sshc/sync_test.go`
- Modify: `cmd/sshc/main.go`

**Interfaces:**
- Produces: `runSync(ctx context.Context, called syncInvocation, stateDir string, client *http.Client, stdin *os.File, stdout, stderr io.Writer, terminal passwordTerminal) int`.
- Produces: human `writeSyncStatus` and JSON success/failure envelope functions.
- Consumes: Tasks 1 and 5 plus `api.SyncStatus`.

- [ ] **Step 1: Write failing status/output tests**

Use a fake engine API client boundary or real `httptest.Server` and require human output to name configured, target, direction, key status, auto phase, last sync, and last operation counts/bytes without credentials. Require JSON success:

```json
{"schemaVersion":1,"success":true,"status":{"configured":true}}
```

Require JSON failure to be exactly one stdout object with `failure.kind` and `retryable`, with empty stderr. Human failure writes actionable stderr and empty stdout. Require cancel to return 130.

- [ ] **Step 2: Verify RED**

Run: `go test ./cmd/sshc -run 'TestSyncStatus|TestSyncJSONFailure|TestRunSyncCanceled' -count=1`

Expected: FAIL because the runner and renderers do not exist.

- [ ] **Step 3: Implement status orchestration and envelopes**

Open the Task 5 client, defer best-effort close, call `GET /api/v1/sync` into `api.SyncStatus`, and render one of the two modes. Use one error classifier for every later sync action:

```go
type commandFailure struct {
	Kind      string `json:"kind"`
	Retryable bool   `json:"retryable"`
}

type commandEnvelope struct {
	SchemaVersion int             `json:"schemaVersion"`
	Success       bool            `json:"success"`
	Status        *api.SyncStatus `json:"status,omitempty"`
	Result        any             `json:"result,omitempty"`
	Failure       *commandFailure `json:"failure,omitempty"`
}
```

Dispatch `invocationSync` from `main.go` under `signal.NotifyContext` so setup/network cancellation maps to 130.

- [ ] **Step 4: Run sync-status and parser regressions**

Run: `go test ./cmd/sshc -run 'TestSyncStatus|TestSyncJSONFailure|TestRunSyncCanceled|TestParseInfoAndSync' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit status**

```bash
git add cmd/sshc/sync.go cmd/sshc/sync_test.go cmd/sshc/main.go
git commit -m "feat(cli): report engine sync status"
```

### Task 7: Secure Interactive Sync Setup

**Files:**
- Create: `cmd/sshc/sync_setup.go`
- Create: `cmd/sshc/sync_setup_test.go`
- Modify: `cmd/sshc/sync.go`
- Modify: `cmd/sshc/vault.go`

**Interfaces:**
- Produces: `runSyncSetup` called by `runSync` for `syncSetup`.
- Produces: bounded one-byte-at-a-time visible-line reader and zeroable setup payload builders.
- Consumes: existing `passwordTerminal.ReadPassword`, `appendVaultJSONString`, `zeroBytes`, Task 5 `sendSecretJSON`, and generated setup response types.

- [ ] **Step 1: Write failing TTY and prompt tests**

Use temporary OS files plus a fake `passwordTerminal` to require both stdin and prompt output to be terminals. Reject pipe input before issuing an API request. Feed visible answers in this order and hidden answers through `ReadPassword`:

```text
Endpoint [https://]:
Bucket:
Path []:
Region [auto]:
Direction [both]:
Access key ID:
Secret access key:
Sync key:              only for an existing target
```

Assert prompt/output and captured errors never contain sentinel access, secret, or existing sync key bytes.

- [ ] **Step 2: Verify TTY tests fail**

Run: `go test ./cmd/sshc -run 'TestSyncSetupRequiresTTY|TestSyncSetupPromptsWithoutEchoingSecrets' -count=1`

Expected: FAIL because setup does not exist.

- [ ] **Step 3: Write failing setup state-machine tests**

Test exact request order and persistence boundary through fake API responses:

- check error: no complete request;
- `incomplete`: no complete request;
- `existing`: hidden key required and exact ETag/history/state copied;
- `empty`: empty key sent and generated key printed once to the prompt terminal;
- target-changed problem: operational failure and no success text;
- cancellation at every prompt: exit 130, no later request;
- oversized/invalid visible or secret input: exit 1 without truncated submission.

Decode captured JSON and require its keys to match `api.SyncSetupCheckRequest` and `api.SyncSetupRequest`. Scan all stdout/stderr/error text for credential sentinels.

- [ ] **Step 4: Verify state-machine tests fail**

Run: `go test ./cmd/sshc -run 'TestSyncSetupChecksBeforeSaving|TestSyncSetupExisting|TestSyncSetupEmpty|TestSyncSetupIncomplete|TestSyncSetupCanceled' -count=1`

Expected: FAIL because the wizard and payload builders do not exist.

- [ ] **Step 5: Implement bounded visible and hidden input**

Read visible TTY input without a buffering reader that could consume the next hidden line. Bound each line at 4 KiB and validate endpoint/bucket/path/region/direction before network access. Use `promptVaultPassword` for hidden values and keep them as `[]byte`; defer `zeroBytes` immediately after every successful read.

- [ ] **Step 6: Implement zeroable wire payloads and setup flow**

Build JSON into fixed-capacity `[]byte` values using the existing byte-oriented JSON string appender for secret fields. Send check first. For existing targets prompt for key; for empty targets send an empty key. Copy only the check response's state/history/ETag into complete. Zero each request body after `Do` returns.

Decode complete into `api.SyncSetupResponse`. Print generated key only to the verified terminal, once, and print a nonsecret target/status summary to stdout.

- [ ] **Step 7: Run setup and vault secret-handling tests**

Run: `go test ./cmd/sshc -run 'TestSyncSetup|TestVaultPassword|TestAppendVaultJSONString|TestOneShot' -count=1`

Expected: PASS.

- [ ] **Step 8: Commit setup**

```bash
git add cmd/sshc/sync_setup.go cmd/sshc/sync_setup_test.go cmd/sshc/sync.go cmd/sshc/vault.go
git commit -m "feat(cli): configure sync from a secure TTY"
```

### Task 8: Push, Pull, Now, and Auto

**Files:**
- Modify: `cmd/sshc/sync.go`
- Modify: `cmd/sshc/sync_test.go`

**Interfaces:**
- Produces: internal `runSyncPush`, `runSyncPull`, `runSyncNow`, and `runSyncAuto` operations.
- Consumes: Task 5 API methods; `api.SyncPushDraft`, `api.PushResponse`, `api.PullResponse`, `api.AutoSyncRequest`, `api.IssueActionRequest`; `session.ActionSyncForcePush`; `remotesync.ForcePushTarget`.

- [ ] **Step 1: Write failing normal push tests**

Require `GET /api/v1/sync/push` followed by `POST /api/v1/sync/push` carrying exactly the returned draft message. A CAS problem exits 1 and is not retried. Human and JSON success report generated response counts and bytes without secrets.

- [ ] **Step 2: Verify push RED, then implement normal push**

Run: `go test ./cmd/sshc -run TestSyncPush -count=1`

Expected before implementation: FAIL because the calls do not occur.

Implement only the two existing API calls, then rerun the same command and expect PASS.

- [ ] **Step 3: Write failing force-push tests**

Require this exact sequence:

```text
POST /api/v1/actions          {"kind":"sync.force_push","target":"live"}
POST /api/v1/sync/force-push  X-SSHC-Action: <issued token>
```

Assert there is no prompt and no retry. Return `sync_remote_moved` on the second call and assert the CLI does not request a second action token or force request.

- [ ] **Step 4: Verify force RED, implement, and rerun**

Run: `go test ./cmd/sshc -run TestSyncForcePush -count=1`

Expected before implementation: FAIL; after using existing action/force API: PASS.

- [ ] **Step 5: Write failing pull preview/apply tests**

Cover four independent cases:

1. safe writes: preview then apply with exact `expectedETag` and `expectedRevision`;
2. normal conflict: preview only, local failure `sync_pull_requires_force`;
3. normal removal: preview only, same refusal;
4. `--force`: preview/apply both use `resolve=remote`, and apply carries exact preview identity.

For a stale apply response, assert no re-preview/retry. For no changes, return success from the preview without an apply request.

- [ ] **Step 6: Verify pull RED, implement, and rerun**

Run: `go test ./cmd/sshc -run TestSyncPull -count=1`

Expected before implementation: FAIL; after orchestration through existing pull API: PASS.

- [ ] **Step 7: Write failing now/auto tests**

Require `POST /api/v1/sync/now` with `{}` and `PUT /api/v1/sync/auto` with exactly `{"enabled":true|false}`. Require returned `api.SyncStatus` in human/JSON output and no local timer/daemon behavior.

- [ ] **Step 8: Verify now/auto RED, implement, and rerun**

Run: `go test ./cmd/sshc -run 'TestSyncNow|TestSyncAuto' -count=1`

Expected before implementation: FAIL; after existing API calls: PASS.

- [ ] **Step 9: Run every sync CLI test**

Run: `go test ./cmd/sshc -run 'TestSync|TestEngineAPI' -count=1`

Expected: PASS.

- [ ] **Step 10: Commit sync operations**

```bash
git add cmd/sshc/sync.go cmd/sshc/sync_test.go
git commit -m "feat(cli): run guarded sync operations"
```

### Task 9: Documentation, Smoke Coverage, and Full Verification

**Files:**
- Modify: `README.md`
- Modify: `docs/headless-examples.md`
- Modify: `docs/manual-acceptance.md`
- Modify: `scripts/ci/cli-smoke.sh`
- Modify: `scripts/ci/cli-smoke.ps1`

**Interfaces:**
- Consumes: all public commands from Tasks 1-8.
- Produces: release-binary smoke coverage and user-facing headless instructions.

- [ ] **Step 1: Write the failing smoke assertions**

In both shell and PowerShell smoke scripts, create a minimal `~/.ssh/config` after the temporary HOME is selected:

```text
Host smoke-info
  HostName 192.0.2.10
  User smoke-user
  Port 22
```

Run `sshc info smoke-info --json`, parse with the platform's existing JSON facility, and require alias/HostName/User/Port/schemaVersion. Do not add S3 credentials or a destructive sync smoke.

- [ ] **Step 2: Build a release-style host binary and run the smoke script**

Run with an explicit temporary output so the user's untracked workspace-root `sshc` file is never a build target:

```bash
SSHC_SMOKE_DIR=$(mktemp -d)
SSHC_SMOKE_OS=$(go env GOOS)
SSHC_SMOKE_ARCH=$(go env GOARCH)
go build -ldflags '-X main.version=plan-test' \
  -o "$SSHC_SMOKE_DIR/sshc-$SSHC_SMOKE_OS-$SSHC_SMOKE_ARCH" ./cmd/sshc
scripts/ci/cli-smoke.sh "$SSHC_SMOKE_DIR" plan-test
```

Expected: the script passes version, no-engine guidance, engine handoff/status, UI, and the new parsed `info` assertions.

- [ ] **Step 3: Update documentation**

Document:

- `sshc info <alias> [--json]` is engine-independent and redacts secret-bearing directives;
- sync requires `sshc engine` plus `sshc vault unlock`;
- `sshc sync setup` requires an interactive terminal and never accepts env/argv credentials;
- normal/force push and pull semantics, including exact-state refusal;
- `sshc sync`, mutation `--json`, `now`, and persistent `auto on|off` examples;
- the one-time generated sync key must be stored safely.

Add manual acceptance rows for Match/Include/ProxyJump info parity, setup non-persistence on failed check, normal pull conflict/removal refusal, force pull remote authority, and force-push remote race refusal.

- [ ] **Step 4: Format and run focused packages**

Run:

```bash
gofmt -w cmd/sshc internal/app internal/session internal/httpserver
scripts/ci/check-gofmt.sh
go test ./cmd/sshc ./internal/app ./internal/session ./internal/httpserver -count=1
go test ./cmd/sshc ./internal/session ./internal/httpserver -race -count=1
```

Expected: all PASS and gofmt reports no diff.

- [ ] **Step 5: Run repository-wide static and unit verification**

Run:

```bash
go vet ./...
go test ./... -count=1
go test ./... -race -count=1
make deadcode
make verify-generated
npm test --prefix web
npm run typecheck --prefix web
```

Expected: all PASS; generated files remain unchanged because no OpenAPI route/schema was added.

- [ ] **Step 6: Cross-build the CLI without installing anything**

Run with every output under an explicit temporary directory:

```bash
SSHC_CROSS_DIR=$(mktemp -d)
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$SSHC_CROSS_DIR/sshc-linux-amd64" ./cmd/sshc
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$SSHC_CROSS_DIR/sshc-linux-arm64" ./cmd/sshc
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o "$SSHC_CROSS_DIR/sshc-windows-amd64.exe" ./cmd/sshc
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -o "$SSHC_CROSS_DIR/sshc-windows-arm64.exe" ./cmd/sshc
GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...
```

Expected: all builds succeed. Leave cleanup until the exact temporary path has been printed and checked; never target the workspace-root `sshc` file.

- [ ] **Step 7: Run non-destructive live checks**

Build to a `mktemp -d` output path, start it with a temporary HOME, create/unlock a temporary vault through a PTY, and verify:

- `info` works before and after engine start;
- `sync` fails with actionable engine/vault guidance in each missing state;
- unlocked `sync` reaches the existing API and reports unconfigured without exposing credentials;
- interrupt returns 130.

Do not configure a real bucket or invoke push/pull/force unless a dedicated disposable target is already available and its exact target has been confirmed read-only first.

- [ ] **Step 8: Review the diff for scope and secrets**

Run:

```bash
git diff --check
git status --short
git diff --stat
git diff -- README.md docs/headless-examples.md docs/manual-acceptance.md cmd/sshc internal/app internal/session internal/httpserver scripts/ci
```

Confirm no credential/token sentinel, generated artifact, untracked `desktop/`, or user `sshc` file is staged.

- [ ] **Step 9: Commit docs and verification changes**

```bash
git add README.md docs/headless-examples.md docs/manual-acceptance.md scripts/ci/cli-smoke.sh scripts/ci/cli-smoke.ps1
git commit -m "docs: explain headless info and sync workflows"
```

- [ ] **Step 10: Follow the branch completion workflow**

Use `superpowers:verification-before-completion`, then `superpowers:finishing-a-development-branch`. Because the user already requested a push, integrate the isolated implementation branch into local `main` without overwriting unrelated files, re-run the required final verification on integrated `main`, fetch/reconcile `origin/main` without force, and push only when the branch is a safe fast-forward.
