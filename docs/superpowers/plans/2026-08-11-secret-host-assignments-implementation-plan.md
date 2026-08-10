# Secret Host Assignments Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show labelled host assignments for every named account password, named key passphrase, and key-dedicated passphrase without exposing a secret value.

**Architecture:** A focused application projection computes a key-path-to-concrete-host index from the current Include graph and effective `IdentityFile` directives. The password HTTP handler joins that read-only index with vault subjects and returns explicit usage metadata; the React Secrets page only renders supplied relationships. A `keyHostUsageComplete` flag distinguishes a confirmed empty key-host list from an unavailable configuration projection without turning a successful vault mutation into an error.

**Tech Stack:** Go 1.25, Echo v5, OpenAPI 3, React 19, TypeScript 5.9, Vitest, Testing Library, Playwright, Docker-backed SeaweedFS and OpenSSH integration tests.

## Global Constraints

- Do not install packages or change dependency versions.
- Never return or render a password, key passphrase, master password, ciphertext, prompt content, or value-derived metadata.
- Keep named account passwords, named key passphrases, and key-dedicated passphrases structurally distinct.
- Host usage is computed from current configuration on every read and is never persisted.
- Expand inherited rules such as `Host *` only across concrete aliases already declared in the loaded graph.
- Do not evaluate `Match exec` or guess relative, tokenised, external, or open-ended host aliases.
- All returned arrays are non-null, sorted, and deduplicated.
- A configuration-projection failure must not make a completed vault mutation appear to have failed.
- Use `apply_patch` for source edits and preserve unrelated worktree changes.

---

## File map

- Create `internal/application/credentialusage.go`: configuration-only key-to-host projection.
- Create `internal/application/credentialusage_test.go`: direct, inherited, deduplicated, and unresolved-path behavior.
- Modify `api/openapi.yaml`: named credential hosts, dedicated passphrase usage, and completeness.
- Regenerate `internal/api/models.gen.go` and `web/src/api/schema.d.ts`: generated contracts.
- Modify `internal/httpserver/password.go`: join vault subjects with host usage without returning values.
- Modify `internal/httpserver/password_test.go`: HTTP contract, incomplete projection, and leak regressions.
- Modify `internal/httpserver/server.go`: inject the configuration projection.
- Modify `web/src/api/integrations.ts` and its test: validate every new response field.
- Modify `web/src/secrets/SecretsPanel.tsx` and its test: labelled keys/hosts and dedicated entries.
- Modify `web/src/i18n/messages.ts`: English and Japanese labels and notices.
- Modify `web/e2e/secrets.spec.ts` and `web/e2e/connections.spec.ts`: built-binary journeys.

---

### Task 1: Project concrete hosts from key paths

**Files:**
- Create: `internal/application/credentialusage.go`
- Create: `internal/application/credentialusage_test.go`

**Interfaces:**
- Consumes: `(*Service).resolve()`, `ProjectHosts`, `ComputeEffective`, `AbsolutePath`, `config.EqualKeyword`, and `keys.ExpandsTo`.
- Produces: `func (s *Service) KeyHosts(relativePaths []string) (map[string][]string, error)`.

- [ ] **Step 1: Write the failing direct and inherited-host test**

Create a fixture that replaces the test workspace `config`, then make hand-derived assertions:

```go
func TestKeyHostsFindsDirectAndInheritedIdentityFiles(t *testing.T) {
	service, workspace := newTestService(t)
	contents := "Host direct\n\tIdentityFile ~/.ssh/keys/id_team\n" +
		"Host inherited-a inherited-b\n\tHostName example.test\n" +
		"Host *\n\tIdentityFile ~/.ssh/keys/id_global\n"
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := service.KeyHosts([]string{"keys/id_team", "keys/id_global", "keys/unused"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got["keys/id_team"], []string{"direct"}) {
		t.Fatalf("team hosts = %#v", got["keys/id_team"])
	}
	if !slices.Equal(got["keys/id_global"], []string{"direct", "inherited-a"}) {
		t.Fatalf("global hosts = %#v", got["keys/id_global"])
	}
	if got["keys/unused"] == nil || len(got["keys/unused"]) != 0 {
		t.Fatalf("unused hosts = %#v, want non-nil empty", got["keys/unused"])
	}
}
```

The second pattern on one `Host` line is not a separate primary connection in the existing projection, so expected concrete aliases are `direct` and `inherited-a`.

- [ ] **Step 2: Run the test and verify RED**

```bash
go test ./internal/application -run '^TestKeyHostsFindsDirectAndInheritedIdentityFiles$' -count=1
```

Expected: compile failure because `(*Service).KeyHosts` does not exist.

- [ ] **Step 3: Write failing deduplication and conservative-path test**

Use this config and assert literal results:

```go
contents := "Host zed\n\tIdentityFile ~/.ssh/id_shared\n\tIdentityFile ~/.ssh/id_shared\n" +
	"Host alpha\n\tIdentityFile ~/.ssh/id_shared\n" +
	"Host zed\n\tIdentityFile ~/.ssh/id_other\n" +
	"Host relative\n\tIdentityFile id_relative\n"
got, err := service.KeyHosts([]string{"id_shared", "id_other", "id_relative"})
// got["id_shared"] == []string{"alpha", "zed"}
// got["id_other"] == []string{"zed"}: cumulative IdentityFile directives in
// both matching Host zed blocks are effective even though the alias is listed once.
// got["id_relative"] is a non-nil empty array.
```

The production change caught by this test is adding the same alias twice, dropping a cumulative directive from a second matching block, or guessing a relative key path.

- [ ] **Step 4: Implement the minimal projection**

Create `credentialusage.go` with this exact boundary:

```go
func (s *Service) KeyHosts(relativePaths []string) (map[string][]string, error) {
	hostsByKey := make(map[string][]string, len(relativePaths))
	absoluteByKey := make(map[string]string, len(relativePaths))
	for _, relative := range relativePaths {
		hostsByKey[relative] = []string{}
		absolute, err := AbsolutePath(s.workspace.Root(), relative)
		if err != nil {
			return nil, err
		}
		absoluteByKey[relative] = absolute
	}
	graph, err := s.resolve()
	if err != nil {
		return nil, err
	}
	projected, _ := ProjectHosts(graph, s.workspace.Root())
	seen := make(map[string]map[string]bool, len(absoluteByKey))
	for relative := range absoluteByKey {
		seen[relative] = map[string]bool{}
	}
	for _, host := range projected {
		alias := host.Identity.Alias
		if alias == "" || host.Duplicate {
			continue
		}
		for _, entry := range ComputeEffective(graph, s.workspace.Root(), alias).Entries {
			if !config.EqualKeyword(entry.Keyword, "IdentityFile") {
				continue
			}
			for _, value := range entry.Values {
				for relative, absolute := range absoluteByKey {
					if keys.ExpandsTo(s.workspace, value, absolute) {
						seen[relative][alias] = true
					}
				}
			}
		}
	}
	for relative, aliases := range seen {
		for alias := range aliases {
			hostsByKey[relative] = append(hostsByKey[relative], alias)
		}
		slices.Sort(hostsByKey[relative])
	}
	return hostsByKey, nil
}
```

Import only `slices`, `sshc/internal/config`, and `sshc/internal/keys`; do not duplicate path expansion.

- [ ] **Step 5: Run focused tests and verify GREEN**

```bash
go test ./internal/application -run '^TestKeyHosts' -count=1
```

Expected: both tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/application/credentialusage.go internal/application/credentialusage_test.go
git commit -m "feat: project key hosts from ssh config"
```

---

### Task 2: Return explicit credential usage through the API

**Files:**
- Modify: `api/openapi.yaml`
- Modify: `internal/api/models.gen.go` (generated)
- Modify: `web/src/api/schema.d.ts` (generated)
- Modify: `internal/httpserver/password.go`
- Modify: `internal/httpserver/password_test.go`
- Modify: `internal/httpserver/server.go`

**Interfaces:**
- Consumes: `(*application.Service).KeyHosts([]string) (map[string][]string, error)` and vault methods `Credentials` and `DedicatedKeyPassphrases`.
- Produces: `Credential.hosts []string`, `DedicatedKeyPassphraseUsage{key, hosts}`, `CredentialList.dedicatedKeyPassphrases`, and `CredentialList.keyHostUsageComplete`.

- [ ] **Step 1: Write failing HTTP contract test**

Extend the password harness to inject:

```go
KeyHosts func([]string) (map[string][]string, error)
```

Write `TestCredentialsListIncludesNamedAndDedicatedHostUsage`. Store and assign an account password, assign `team-phrase` to `keys/id_a` and `keys/id_b`, and create a dedicated passphrase for `keys/id_owned` through `WithConnectionSecretsMutation`. Return this literal map from the injected callback:

```go
map[string][]string{
	"keys/id_a":     {"build-a"},
	"keys/id_b":     {"build-a", "build-b"},
	"keys/id_owned": {"deploy"},
}
```

Decode `api.CredentialList` and assert these exact relationships:

```text
office: Uses=[web-1], Hosts=[web-1]
team-phrase: Uses=[keys/id_a, keys/id_b], Hosts=[build-a, build-b]
dedicated: Key=keys/id_owned, Hosts=[deploy]
keyHostUsageComplete=true
```

Scan the response for the stored password, shared passphrase, dedicated passphrase, and master password; each must be absent.

- [ ] **Step 2: Run the HTTP test and verify RED**

```bash
go test ./internal/httpserver -run '^TestCredentialsListIncludesNamedAndDedicatedHostUsage$' -count=1
```

Expected: compile failure because the callback and response fields do not exist.

- [ ] **Step 3: Extend OpenAPI and regenerate**

Use this exact schema contract:

```yaml
Credential:
  type: object
  additionalProperties: false
  required: [kind, name, uses, hosts]
  properties:
    kind: { type: string }
    name: { type: string }
    uses: { type: array, items: { type: string } }
    hosts: { type: array, items: { type: string } }
DedicatedKeyPassphraseUsage:
  type: object
  additionalProperties: false
  required: [key, hosts]
  properties:
    key: { type: string }
    hosts: { type: array, items: { type: string } }
CredentialList:
  type: object
  additionalProperties: false
  required: [credentials, dedicatedKeyPassphrases, keyHostUsageComplete]
  properties:
    credentials: { type: array, items: { $ref: "#/components/schemas/Credential" } }
    dedicatedKeyPassphrases: { type: array, items: { $ref: "#/components/schemas/DedicatedKeyPassphraseUsage" } }
    keyHostUsageComplete: { type: boolean }
```

Run:

```bash
make generate
```

Expected: both generated models contain required fields with no optional markers.

- [ ] **Step 4: Implement the handler join and server wiring**

Add `KeyHosts` to `PasswordHandlers`. In `listCredentials`, collect every named key-passphrase subject and dedicated key path, sort/deduplicate, and call the callback once.

- Password credentials receive `Hosts: slices.Clone(Uses)`.
- Named passphrases receive the sorted union of mapped hosts for every key in `Uses`.
- Dedicated items receive their one key and its mapped hosts.
- Every response collection is initialized to `[]`, never `nil`.
- With no key paths, usage is complete without calling the callback.
- A nil callback with key paths, or a callback error, produces empty key-host arrays and `KeyHostUsageComplete: false`, while retaining HTTP 200 and every vault relationship. Password hosts remain populated from vault uses.

Wire production with:

```go
var credentialHosts func([]string) (map[string][]string, error)
if options.Config != nil {
	credentialHosts = options.Config.KeyHosts
}
registerPasswordRoutes(e, PasswordHandlers{
	Service:          options.Passwords,
	KeyHosts:        credentialHosts,
	Answerable:       options.Answerable,
	Eligibility:      eligibility,
	ResealSnapshot:   reseal,
})
```

- [ ] **Step 5: Add the mutation-truthfulness regression test**

Inject `errors.New("broken include graph")`, perform `PUT /api/v1/credentials/password/office`, and assert:

```go
if response.Code != http.StatusOK {
	t.Fatalf("stored credential reported as failed: %d %s", response.Code, response.Body.String())
}
if answer.KeyHostUsageComplete {
	t.Fatal("failed projection reported complete")
}
```

This catches the bug where a completed encrypted write is reported as failed because the following read-only configuration projection failed.

- [ ] **Step 6: Run contract and package tests**

```bash
go test ./internal/api ./internal/httpserver ./internal/application -count=1
make generate
git diff --check
```

Expected: all tests pass, regeneration succeeds, and the intended generated contract diff has no whitespace errors.

- [ ] **Step 7: Commit**

```bash
git add api/openapi.yaml internal/api/models.gen.go web/src/api/schema.d.ts internal/httpserver/password.go internal/httpserver/password_test.go internal/httpserver/server.go
git commit -m "feat: expose credential host assignments"
```

- [ ] **Step 8: Verify committed generated files**

```bash
make verify-generated
```

Expected: regeneration produces no change from the newly committed generated files.

---

### Task 3: Render labelled named and dedicated assignments

**Files:**
- Modify: `web/src/api/integrations.ts`
- Modify: `web/src/api/integrations.test.ts`
- Modify: `web/src/secrets/SecretsPanel.tsx`
- Modify: `web/src/secrets/SecretsPanel.test.tsx`
- Modify: `web/src/i18n/messages.ts`

**Interfaces:**
- Consumes: generated `CredentialList`, `Credential.hosts`, `dedicatedKeyPassphrases`, `keyHostUsageComplete`, and existing `unassignCredential("key_passphrase", key)`.
- Produces: labelled host/key lists, dedicated removal, explicit empty states, and an incomplete-usage notice.

- [ ] **Step 1: Write failing API-client validation tests**

Update the valid fixture to include:

```ts
{
  credentials: [{ kind: "password", name: "office", uses: ["web-1"], hosts: ["web-1"] }],
  dedicatedKeyPassphrases: [{ key: "keys/id_owned", hosts: ["deploy"] }],
  keyHostUsageComplete: true,
}
```

Add malformed fixtures with missing `hosts`, non-array `dedicatedKeyPassphrases`, a non-string dedicated `key`, and non-boolean `keyHostUsageComplete`. Each must reject with `invalid_response`.

- [ ] **Step 2: Run API-client tests and verify RED**

```bash
npm test --prefix web -- src/api/integrations.test.ts
```

Expected: the valid contract or malformed-field rejection fails because new fields are not validated.

- [ ] **Step 3: Implement strict response validation**

Extend `validateCredentialList`:

```ts
for (const host of asArray(entry.hosts)) asString(host);
for (const dedicated of asArray(record.dedicatedKeyPassphrases)) {
  const item = asRecord(dedicated);
  asString(item.key);
  for (const host of asArray(item.hosts)) asString(host);
}
asBoolean(record.keyHostUsageComplete);
```

Run the focused API test again and require it to pass.

- [ ] **Step 4: Write failing Secrets page rendering tests**

Use a complete fixture:

```ts
credentials: [
  { kind: "password", name: "office-vm", uses: ["web-1", "web-2"], hosts: ["web-1", "web-2"] },
  { kind: "password", name: "unused", uses: [], hosts: [] },
  { kind: "key_passphrase", name: "build-key", uses: ["keys/id_a", "keys/id_b"], hosts: ["build-a", "build-b"] },
],
dedicatedKeyPassphrases: [{ key: "keys/id_owned", hosts: ["deploy"] }],
keyHostUsageComplete: true,
```

Assert behavior within each credential item:

```ts
expect(within(office).getByText("Assigned hosts")).toBeInTheDocument();
expect(within(office).getByRole("list", { name: "Assigned hosts" })).toHaveTextContent("web-1");
expect(within(build).getByRole("list", { name: "Keys" })).toHaveTextContent("keys/id_a");
expect(within(build).getByRole("list", { name: "Assigned hosts" })).toHaveTextContent("build-b");
expect(within(dedicated).getByText("Dedicated to this key")).toBeInTheDocument();
```

Add separate tests for named empty hosts, incomplete usage text, and clicking `Remove saved passphrase for keys/id_owned`, which must call:

```ts
expect(api.unassignCredential).toHaveBeenCalledWith("key_passphrase", "keys/id_owned");
```

- [ ] **Step 5: Run Secrets tests and verify RED**

```bash
npm test --prefix web -- src/secrets/SecretsPanel.test.tsx
```

Expected: labelled lists and dedicated entry are absent.

- [ ] **Step 6: Implement UI and translations**

Store the full `CredentialList` response instead of only `response.credentials`. Render each entry in an `article` labelled by its credential name or dedicated key name, with a stacked layout:

- password: name, assigned hosts, delete;
- named passphrase: name, keys, assigned hosts, delete;
- dedicated passphrase: key basename plus dedicated marker, one-key list, assigned hosts, dedicated removal.

Add these English/Japanese message pairs:

```text
secrets.assignedHosts = Assigned hosts / 割り当て先ホスト
secrets.noAssignedHosts = No assigned hosts / 割り当て先ホストはありません
secrets.keys = Keys / 対象の鍵
secrets.noKeys = No assigned keys / 対象の鍵はありません
secrets.dedicated = Dedicated to this key / この鍵専用
secrets.removeDedicated = Remove saved passphrase for {key} / {key} の保存済みパスフレーズを削除
secrets.hostUsageIncomplete = Host assignments could not be fully confirmed from SSH configuration. Check Config diagnostics. / SSH設定を読み切れなかったため、割り当て先ホストを完全に確認できませんでした。設定画面の診断を確認してください。
```

When `keyHostUsageComplete` is false, render the notice and show only key-derived host lists as unavailable rather than empty. Keep password hosts and key paths visible because they come from the unlocked vault. Count dedicated entries in both the passphrase metric and assignment metric; do not count derived hosts as vault assignments.

- [ ] **Step 7: Run focused Web tests and typecheck**

```bash
npm test --prefix web -- src/api/integrations.test.ts src/secrets/SecretsPanel.test.tsx
npm run typecheck --prefix web
```

Expected: focused tests and both TypeScript projects pass.

- [ ] **Step 8: Commit**

```bash
git add web/src/api/integrations.ts web/src/api/integrations.test.ts web/src/secrets/SecretsPanel.tsx web/src/secrets/SecretsPanel.test.tsx web/src/i18n/messages.ts
git commit -m "feat: show hosts for stored secrets"
```

---

### Task 4: Prove the complete user journey and publish

**Files:**
- Modify: `web/e2e/secrets.spec.ts`
- Modify: `web/e2e/connections.spec.ts`
- Modify: `internal/ui/dist` assets produced by `make build`

**Interfaces:**
- Consumes: complete API and Secrets UI from Tasks 1-3.
- Produces: built-binary coverage for named passwords and dedicated key passphrases, a clean generated bundle, and a verified push.

- [ ] **Step 1: Write built-binary E2E assertions**

In the named password journey, replace the position-based `bastion, nas` assertion with checks for the `Assigned hosts` list containing both aliases.

In the existing connection-owned key-passphrase journey, after saving the dedicated passphrase and opening Secrets, assert one dedicated item contains the English equivalents of:

```text
この鍵専用
id_connection_owned
bastion
```

Also assert that the passphrase plaintext is absent from `body`.

- [ ] **Step 2: Build and run focused E2E**

```bash
make build
npm run e2e --prefix web -- secrets.spec.ts connections.spec.ts
```

Expected: focused E2E passes against the built binary and no plaintext secret appears. Production behavior already completed its failing-test cycle in Tasks 1-3; these tests add boundary coverage rather than another production change.

- [ ] **Step 3: Run complete local verification**

```bash
make test
make e2e
make verify-generated
git diff --check
```

Expected: all Go, race, Web, typecheck, E2E, and generated-contract checks pass. The existing Vite chunk-size message may remain a non-failing warning; any new warning or test failure is fixed.

- [ ] **Step 4: Run Docker integration with guaranteed cleanup**

```bash
trap 'make integration-down' EXIT
make integration-up
make integration
make integration-down
trap - EXIT
test ! -e .integration-s3.json
test -z "$(docker ps -a --filter name=sshc-s3 --filter name=sshc-sshd --format '{{.Names}}')"
```

Expected: real S3 and sshd tests pass, the fixture is removed, and neither container remains.

- [ ] **Step 5: Commit E2E and generated assets**

```bash
git add web/e2e/secrets.spec.ts web/e2e/connections.spec.ts internal/ui/dist
git commit -m "test: cover secret host assignments end to end"
```

- [ ] **Step 6: Review and push main**

```bash
git status --short --branch
git diff --check
git log --oneline origin/main..HEAD
git fetch origin main
test "$(git rev-parse origin/main)" = "$(git merge-base HEAD origin/main)"
git push origin main
```

Expected: worktree is clean before push, upstream has not diverged, and push succeeds without force.

- [ ] **Step 7: Verify CI for the exact pushed SHA**

```bash
head_sha=$(git rev-parse HEAD)
run_id=$(gh run list --branch main --commit "$head_sha" --limit 1 --json databaseId --jq '.[0].databaseId')
test -n "$run_id"
gh run watch "$run_id" --exit-status
git fetch origin main
test "$head_sha" = "$(git rev-parse origin/main)"
git status --short --branch
```

Expected: Web, Go, macOS, dependency security, generated files, E2E, and Docker integration all succeed for the exact local SHA; local `main` equals `origin/main` and remains clean.
