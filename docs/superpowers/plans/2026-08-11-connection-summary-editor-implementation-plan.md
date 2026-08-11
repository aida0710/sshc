# Connection Summary Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the existing connection detail as a saved-state summary, explicit connection checks, an always-available Basic editor, a settings-analysis view, and grouped advanced/management actions without losing drafts or weakening the application vault gate.

**Architecture:** `ConnectionsPage` remains the owner of the selected identity, committed server snapshot, terminal metadata, and mutations. A partial-failure-aware saved-state loader supplies both `ConnectionSummary` and `ConnectionBasicForm`; the detail panel keeps typed drafts mounted across view changes and reports one active dirty domain upward. Route compatibility stays on the existing `tab=` vocabulary while a view mapper presents only Basic, Settings analysis, and Advanced settings.

**Tech Stack:** React 19, TypeScript 5.9, Vite 8, Tailwind CSS 4, Vitest, Testing Library, Playwright, Go 1.25 integration harness, Docker-backed SeaweedFS and OpenSSH tests.

## Global Constraints

- Do not install packages or change dependency versions.
- Keep the existing desktop two-pane Connections layout; do not add a smartphone-specific or responsive redesign.
- Keep the global application/vault gate intact; do not add a gate-exempt connection-update route.
- Never put a draft, password, key passphrase, secret value, or check result in URL, history state, localStorage, sessionStorage, logs, or server responses.
- Never start reachability, authentication, `ssh -G`, or terminal launch when a panel or deep link is opened.
- Keep account passwords and private-key passphrases independent in data and presentation.
- Disable Connect, reachability, authentication, and identity/base-changing management while an editor is dirty.
- Preserve the existing transactional `/api/v1/connections` update for combined config and secret changes.
- Treat the completed mutation as the save-success boundary; a later refresh failure must say the save completed and must clear secret drafts.
- A failed credential or key-inventory read must not be converted to an empty assignment or an implicit removal.
- Use `apply_patch` for source edits and preserve unrelated worktree changes.

---

## File map

- Modify `web/src/routing/connectionRoute.ts` and its test: map legacy `tab=` values to the three visible detail areas and Advanced subviews.
- Modify `web/src/routing/useSectionRoute.ts`, `web/src/App.tsx`, and their tests: register an in-memory navigation blocker that also restores a rejected popstate URL.
- Create `web/src/connections/connectionSavedState.ts` and its test: partial-failure-safe loader and safe summary projection.
- Create `web/src/connections/ConnectionSummary.tsx` and its test: committed alias, endpoint, key/passphrase/password/group status, terminal choice, Connect, and Manage entry.
- Modify `web/src/connections/ConnectionBasicForm.tsx` and its test: consume the shared resources, keep config-only editing alive after auxiliary failures, expose dirty/discard state, and send unchanged secret actions.
- Create `web/src/connections/ConnectionChecks.tsx` and its test: explicit reachability and authentication actions with an executable-directive preflight.
- Create `web/src/connections/ConnectionAnalysis.tsx` and its test: explanatory projection plus explicit authoritative `ssh -G` execution.
- Create `web/src/connections/AdvancedSettings.tsx` and its test: Jump, directives, and Raw internal views with one dirty domain.
- Create `web/src/connections/ManageConnection.tsx` and its test: rename, group, comment, duplicate, file move, and delete.
- Modify `web/src/connections/HostDetail.tsx` and its test: assemble the three visible areas while keeping editor components mounted.
- Modify `web/src/connections/ConnectionsPage.tsx` and its test: load one saved state, coordinate save/refresh truthfulness, navigation guards, summary actions, and management mutations.
- Modify `web/src/i18n/messages.ts`: exact English and Japanese labels, statuses, confirmations, and refresh errors.
- Modify `web/e2e/connections.spec.ts` and `web/e2e/routing.spec.ts`: built-binary journeys and legacy deep links.

---

### Task 1: Map legacy URLs and block dirty navigation

**Files:**
- Modify: `web/src/routing/connectionRoute.ts`
- Modify: `web/src/routing/connectionRoute.test.ts`
- Modify: `web/src/routing/useSectionRoute.ts`
- Modify: `web/src/routing/useSectionRoute.test.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.test.tsx`

**Interfaces:**
- Produces: `ConnectionArea`, `AdvancedArea`, `connectionAreaForTab(tab)`, `tabForConnectionArea(area, advanced)`, and `checksExpandedForTab(tab)`.
- Produces: `NavigationBlocker = (next: BrowserLocation) => boolean` and `setNavigationBlocker(blocker)` from `useSectionRoute`.
- Consumes later: `ConnectionsPage` registers a blocker only while dirty; tabs for the same identity remain navigable.

- [ ] **Step 1: Write the failing URL mapping tests**

Add literal compatibility assertions:

```ts
expect(connectionAreaForTab("Basic")).toEqual({ area: "Basic", advanced: "Jump" });
expect(connectionAreaForTab("Diagnostics")).toEqual({ area: "Basic", advanced: "Jump" });
expect(checksExpandedForTab("Diagnostics")).toBe(true);
expect(connectionAreaForTab("Effective")).toEqual({ area: "Analysis", advanced: "Jump" });
expect(connectionAreaForTab("Jump")).toEqual({ area: "Advanced", advanced: "Jump" });
expect(connectionAreaForTab("Advanced")).toEqual({ area: "Advanced", advanced: "Directives" });
expect(connectionAreaForTab("Raw")).toEqual({ area: "Advanced", advanced: "Raw" });
expect(tabForConnectionArea("Analysis", "Jump")).toBe("Effective");
expect(tabForConnectionArea("Advanced", "Raw")).toBe("Raw");
```

- [ ] **Step 2: Run the route test and verify RED**

```bash
npm test --prefix web -- --run src/routing/connectionRoute.test.ts
```

Expected: compile failure because the mapping exports do not exist.

- [ ] **Step 3: Implement the pure mapping without changing URL slugs**

Add these exact types and exhaustive switches while retaining `hostEditorTabs`, `parseConnectionSearch`, and `connectionLocation`:

```ts
export type ConnectionArea = "Basic" | "Analysis" | "Advanced";
export type AdvancedArea = "Jump" | "Directives" | "Raw";

export function connectionAreaForTab(tab: HostEditorTab): {
  area: ConnectionArea;
  advanced: AdvancedArea;
} {
  switch (tab) {
    case "Basic":
    case "Diagnostics": return { area: "Basic", advanced: "Jump" };
    case "Effective": return { area: "Analysis", advanced: "Jump" };
    case "Jump": return { area: "Advanced", advanced: "Jump" };
    case "Advanced": return { area: "Advanced", advanced: "Directives" };
    case "Raw": return { area: "Advanced", advanced: "Raw" };
  }
}
```

`tabForConnectionArea("Basic", ...)` always returns `Basic`; opening checks changes it to `Diagnostics`. No new query keys are introduced.

- [ ] **Step 4: Write failing navigation-blocker tests**

Register a blocker that returns false, call `navigate("Keys")`, and assert pathname and hook location remain `/connections?...`. Then push `/history`, dispatch `popstate`, and assert the hook restores the previous Connections URL using `replaceState`, without publishing `/history` to consumers. Add an allow case that proves navigation proceeds after the blocker is removed.

```ts
act(() => result.current.setNavigationBlocker(() => false));
act(() => result.current.navigate("Keys"));
expect(window.location.pathname).toBe("/connections");

act(() => {
  window.history.pushState(null, "", "/history");
  window.dispatchEvent(new PopStateEvent("popstate"));
});
expect(result.current.location).toEqual(previous);
```

- [ ] **Step 5: Implement the in-memory blocker and App wiring**

Use a ref, not history state or storage:

```ts
export type NavigationBlocker = (next: BrowserLocation) => boolean;
const blocker = useRef<NavigationBlocker | null>(null);
const locationRef = useRef(location);

const setNavigationBlocker = useCallback((next: NavigationBlocker | null) => {
  blocker.current = next;
}, []);
```

Before `navigate` or `navigateLocation` mutates history, return false when blocked. In `popstate`, if rejected, `replaceState` the URL from `locationRef.current` and leave React location unchanged. App passes `setNavigationBlocker` to `ConnectionsPage`; no other section registers one.

- [ ] **Step 6: Run focused route and shell tests**

```bash
npm test --prefix web -- --run src/routing/connectionRoute.test.ts src/routing/useSectionRoute.test.tsx src/App.test.tsx
```

Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add web/src/routing/connectionRoute.ts web/src/routing/connectionRoute.test.ts web/src/routing/useSectionRoute.ts web/src/routing/useSectionRoute.test.tsx web/src/App.tsx web/src/App.test.tsx
git commit -m "feat: guard dirty connection routes"
```

---

### Task 2: Load and present one safe saved state

**Files:**
- Create: `web/src/connections/connectionSavedState.ts`
- Create: `web/src/connections/connectionSavedState.test.ts`
- Create: `web/src/connections/ConnectionSummary.tsx`
- Create: `web/src/connections/ConnectionSummary.test.tsx`

**Interfaces:**
- Consumes: `HostDetail`, `KeysApi.inventory`, and integration methods `passwordVault`, `passwordEligibility`, and `credentials`.
- Produces: `Loadable<T>`, `ConnectionSavedState`, `loadConnectionSavedState(detail, keys, secrets)`, and `connectionSummary(saved)`.
- Produces: `ConnectionSummary` props for committed state, dirty/refetch state, terminal controls, Connect, and Manage.

- [ ] **Step 1: Write the failing partial-load tests**

Use deferred or rejected API mocks and assert the loader never collapses unrelated resources:

```ts
const saved = await loadConnectionSavedState(detail, keys, secrets);
expect(saved.keys.status).toBe("failed");
expect(saved.vault).toEqual({ status: "ready", value: unlockedVault });
expect(saved.credentials).toEqual({ status: "ready", value: credentials });
expect(saved.eligibility.status).toBe("ready");
```

Add the inverse credentials failure and assert the summary returns `accountPassword: { state: "unavailable" }`, never `none`. Add direct known key, custom key, duplicate key, dedicated passphrase, named passphrase, and confirmed-no-password cases.

- [ ] **Step 2: Run the state test and verify RED**

```bash
npm test --prefix web -- --run src/connections/connectionSavedState.test.ts
```

Expected: module-not-found failure.

- [ ] **Step 3: Implement settled resource loading and pure projection**

Use this discriminated union:

```ts
export type Loadable<T> =
  | { status: "loading" }
  | { status: "ready"; value: T }
  | { status: "failed" };

export type ConnectionSavedState = {
  detail: HostDetail;
  keys: Loadable<KeyItem[]>;
  vault: Loadable<PasswordVaultStatus>;
  credentials: Loadable<Credential[]>;
  eligibility: Loadable<PasswordEligibility>;
};
```

Call `Promise.allSettled` for inventory, vault, and eligibility. Call `credentials()` only after an unlocked vault result. A locked vault produces a non-ready credentials state; a rejected credential call produces `failed`. Filter inventory through `selectablePrivateKeys` once. Projection functions may expose names, paths, fingerprints, encryption flags, and assignments, but never values.

- [ ] **Step 4: Write the failing summary component tests**

Render committed `ops@203.0.113.10:2222`, key `id_work — SHA256:test`, a dedicated key-passphrase label, named password `office`, and group `work`. Mutate only the test's draft/dirty prop and prove the endpoint text does not change. Assert `Connect`, `Check reachability`, and `Check authentication with saved settings` are disabled with an accessible reason while dirty.

- [ ] **Step 5: Implement `ConnectionSummary`**

Render one labelled summary card and separate Authentication rows. Props are callbacks and safe metadata only:

```ts
type ConnectionSummaryProps = {
  state: ConnectionSavedState;
  dirty: boolean;
  refreshing: boolean;
  terminal: ReactNode;
  checksExpanded: boolean;
  onToggleChecks: () => void;
  onConnect: () => void;
  connecting: boolean;
  onToggleManage: () => void;
};
```

Do not render a hidden secret value. Use `aria-describedby` for the dirty-state reason instead of relying on disabled-button hover text.

- [ ] **Step 6: Run focused tests and commit**

```bash
npm test --prefix web -- --run src/connections/connectionSavedState.test.ts src/connections/ConnectionSummary.test.tsx
git add web/src/connections/connectionSavedState.ts web/src/connections/connectionSavedState.test.ts web/src/connections/ConnectionSummary.tsx web/src/connections/ConnectionSummary.test.tsx
git commit -m "feat: show committed connection summary"
```

---

### Task 3: Make Basic a persistent, partial-failure-safe editor

**Files:**
- Modify: `web/src/connections/ConnectionBasicForm.tsx`
- Modify: `web/src/connections/ConnectionBasicForm.test.tsx`

**Interfaces:**
- Consumes: `ConnectionSavedState` instead of issuing inventory/vault/credential reads internally.
- Produces: `onDirtyChange(dirty)`, `onDiscardReady(discard)`, and `onRequestRefresh()` contracts for the detail owner.
- Preserves: `onSave(UpdateConnectionRequest): Promise<void>` and secret-clearing behavior.

- [ ] **Step 1: Write failing tests for shared resources and partial failure**

Replace fetch-oriented harness setup with a `state` prop. Test a failed credentials resource with ready key/vault/eligibility resources:

```ts
await user.clear(screen.getByLabelText("Host name or IP address"));
await user.type(screen.getByLabelText("Host name or IP address"), "retry.example");
await user.click(screen.getByRole("button", { name: "Save Basic settings" }));
expect(onSave).toHaveBeenCalledWith(expect.objectContaining({
  hostName: { action: "set", value: "retry.example" },
  password: { kind: "unchanged" },
  keyPassphrase: { kind: "unchanged" },
}));
```

Assert password mutation controls say unavailable and do not offer Remove. With failed key inventory, assert the saved IdentityFile is shown but cannot be changed and is not sent as inherit.

- [ ] **Step 2: Run the Basic test and verify RED**

```bash
npm test --prefix web -- --run src/connections/ConnectionBasicForm.test.tsx
```

Expected: prop/type failures before implementation.

- [ ] **Step 3: Remove internal resource fetches and derive save eligibility by mutation kind**

Replace the single `loading`/`vault.unlocked` gate with:

```ts
const changesSecret = passwordChange.kind !== "unchanged" || keyPassphraseChange.kind !== "unchanged";
const resourcesPermitSecrets = state.vault.status === "ready" && state.vault.value.unlocked &&
  state.credentials.status === "ready" && state.eligibility.status === "ready";
const canSave = !busy && dirty && validFields &&
  (!changesSecret || resourcesPermitSecrets) && passwordAllowed && keyPassphraseValid;
```

Every config-only request must include exactly:

```ts
password: { kind: "unchanged" },
keyPassphrase: { kind: "unchanged" },
```

Never construct `remove` from a missing resource. Report dirty changes after every render and expose a discard callback that restores committed fields and clears all password/passphrase inputs.

- [ ] **Step 4: Add persistent-mount and discard tests**

Rerender the form hidden, then visible, and assert its draft remains. Invoke the registered discard callback and assert HostName returns to the committed value while password/passphrase fields are blank. Assert rejected saves preserve non-secret fields but clear secrets.

- [ ] **Step 5: Run the Basic suite and commit**

```bash
npm test --prefix web -- --run src/connections/ConnectionBasicForm.test.tsx
git add web/src/connections/ConnectionBasicForm.tsx web/src/connections/ConnectionBasicForm.test.tsx
git commit -m "feat: keep connection basic drafts available"
```

---

### Task 4: Separate checks, analysis, and advanced settings

**Files:**
- Create: `web/src/connections/ConnectionChecks.tsx`
- Create: `web/src/connections/ConnectionChecks.test.tsx`
- Create: `web/src/connections/ConnectionAnalysis.tsx`
- Create: `web/src/connections/ConnectionAnalysis.test.tsx`
- Create: `web/src/connections/AdvancedSettings.tsx`
- Create: `web/src/connections/AdvancedSettings.test.tsx`
- Modify: `web/src/connections/HostDetail.tsx`
- Modify: `web/src/connections/HostDetail.test.tsx`

**Interfaces:**
- `ConnectionChecks({ alias, api, disabled, resetKey })` owns only reachability/authentication results.
- `ConnectionAnalysis({ detail, alias, api })` renders explained values and owns explicit authoritative evaluation.
- `AdvancedSettings` consumes field/raw callbacks and reports dirty/discard for one subeditor.
- `HostDetailPanel` presents only `Basic`, `Settings analysis`, and `Advanced settings` tabs while preserving legacy URL callbacks.

- [ ] **Step 1: Write failing check tests**

Assert mount and `expanded=true` make zero API calls. Reachability calls only `api.reachability(alias)`. Authentication first calls `api.effective(alias, false)`; with no executable directives it then calls `api.authentication(alias, false)`. With executable directives it renders their keyword/path/line/command and does not authenticate until the explicit acknowledgement button is clicked, then calls `api.authentication(alias, true)`.

Change `resetKey` or alias and assert old results disappear.

- [ ] **Step 2: Implement `ConnectionChecks` and verify GREEN**

Use separate result/error state for reachability and authentication so one failure does not erase the other. The authentication preflight is:

```ts
const inspection = await api.effective(alias, false);
if (inspection.executableDirectives.length > 0) {
  setPendingDirectives(inspection.executableDirectives);
  return;
}
setAuthentication(await api.authentication(alias, false));
```

No effect starts an operation.

```bash
npm test --prefix web -- --run src/connections/ConnectionChecks.test.tsx
```

- [ ] **Step 3: Write and implement analysis tests**

Assert the saved `detail.effective.entries` and notices render immediately without an API call. Clicking `Run authoritative ssh -G` calls `api.effective(alias, false)`. If confirmation is required, render exact executable directives and call `api.effective(alias, true)` only after `Run anyway`.

```bash
npm test --prefix web -- --run src/connections/ConnectionAnalysis.test.tsx
```

- [ ] **Step 4: Extract Advanced settings with dirty-domain tests**

Move Jump fields, arbitrary directives, and Raw editor from `HostDetail`. Internal buttons map to legacy `Jump`, `Advanced`, and `Raw` URL values. Keep all three editors mounted; `hidden` controls visibility. While one subeditor is dirty, controls in the other two are disabled and explain which draft must be saved or discarded. Save and discard operate only on the active subeditor.

```ts
expect(onDirtyChange).toHaveBeenLastCalledWith({ domain: "raw", dirty: true });
expect(screen.getByLabelText("ProxyJump")).toBeDisabled();
await user.click(screen.getByRole("button", { name: "Discard changes" }));
expect(screen.getByLabelText(/Block text/)).toHaveValue(detail.form.raw);
```

- [ ] **Step 5: Reassemble `HostDetailPanel` and test legacy views**

`HostDetailPanel` maps the controlled legacy tab to one visible tab and Advanced subview. It always mounts Basic, Analysis, and Advanced children under hidden wrappers. A `Diagnostics` deep link selects Basic and expands checks but makes no API call. Remove rename/group/comment management from this component.

- [ ] **Step 6: Run focused suites and commit**

```bash
npm test --prefix web -- --run src/connections/ConnectionChecks.test.tsx src/connections/ConnectionAnalysis.test.tsx src/connections/AdvancedSettings.test.tsx src/connections/HostDetail.test.tsx
git add web/src/connections/ConnectionChecks.tsx web/src/connections/ConnectionChecks.test.tsx web/src/connections/ConnectionAnalysis.tsx web/src/connections/ConnectionAnalysis.test.tsx web/src/connections/AdvancedSettings.tsx web/src/connections/AdvancedSettings.test.tsx web/src/connections/HostDetail.tsx web/src/connections/HostDetail.test.tsx
git commit -m "feat: reorganize connection settings views"
```

---

### Task 5: Assemble management, save truthfulness, and draft guards

**Files:**
- Create: `web/src/connections/ManageConnection.tsx`
- Create: `web/src/connections/ManageConnection.test.tsx`
- Modify: `web/src/connections/ConnectionsPage.tsx`
- Modify: `web/src/connections/ConnectionsPage.test.tsx`

**Interfaces:**
- `ManageConnection` receives committed detail/groups/files, `disabled`, and explicit rename/group/comment/duplicate/move/delete callbacks.
- `ConnectionsPage` owns `ConnectionSavedState`, `editorDirty`, registered discard callback, `refreshingAfterSave`, and `savedRevision`.
- `ConnectionsPage` registers `NavigationBlocker` through the new prop from App.

- [ ] **Step 1: Write failing management tests**

Render all management operations under one labelled region. With `disabled=true`, assert rename, group move, duplicate, file move, and delete are disabled while fields explain the dirty draft. Comment/rename/group state resets when the committed identity/base changes. Delete requires the existing second click.

- [ ] **Step 2: Implement `ManageConnection`**

Keep every operation independent. Do not combine callbacks or mutate the committed snapshot locally. The component may hold input text, selected group/file, and delete confirmation only.

- [ ] **Step 3: Write page tests for one load and committed summary**

Open one host and assert inventory, vault, eligibility, and credentials are each fetched once for the summary and Basic form together. Edit HostName and assert:

```ts
expect(screen.getByText("ops@old.example:2222")).toBeInTheDocument();
expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
expect(screen.getByRole("button", { name: "Connect" })).toBeDisabled();
expect(screen.getByRole("region", { name: "Manage connection" }))
  .toHaveAttribute("aria-disabled", "true");
```

Switch Settings analysis and back; assert the HostName draft remains.

- [ ] **Step 4: Implement atomic refresh orchestration**

After `/api/v1/connections` commits, clear secret inputs immediately, then load overview, detail, and safe metadata. Do not replace the displayed committed summary until the new detail and credential metadata are confirmed. On full success, increment `savedRevision`, replace state, clear dirty/check results, and re-enable actions.

On a post-commit refresh failure:

```ts
setRefreshState("failed");
setLocalError(t("conn.savedRefreshFailed"));
```

Keep the operation classified as saved, wipe secret draft state, and disable Connect/check/manage until an explicit Reload succeeds. Never show `conn.basicSaveFailed` for this path.

- [ ] **Step 5: Add dirty navigation and metadata regressions**

Stub `window.confirm`. Rejecting another host, sidebar section, or popstate keeps identity, URL, and draft. Accepting discards and navigates. A same-identity tab switch does not prompt. Install `beforeunload` only while dirty.

Trigger HostInspector metadata save and terminal selection while Basic is dirty. Assert the Basic value and dirty state remain, and summary endpoint remains committed. Metadata completion must not overwrite the connection snapshot.

- [ ] **Step 6: Run page tests and commit**

```bash
npm test --prefix web -- --run src/connections/ManageConnection.test.tsx src/connections/ConnectionsPage.test.tsx src/App.test.tsx
git add web/src/connections/ManageConnection.tsx web/src/connections/ManageConnection.test.tsx web/src/connections/ConnectionsPage.tsx web/src/connections/ConnectionsPage.test.tsx
git commit -m "feat: assemble guarded connection editor"
```

---

### Task 6: Finish copy, end-to-end coverage, and repository verification

**Files:**
- Modify: `web/src/i18n/messages.ts`
- Modify: `web/e2e/connections.spec.ts`
- Modify: `web/e2e/routing.spec.ts`
- Modify: `web/src/connections/ConnectionSummary.test.tsx`
- Modify: `web/src/connections/ConnectionBasicForm.test.tsx`
- Modify: `web/src/connections/ConnectionChecks.test.tsx`
- Modify: `web/src/connections/ConnectionAnalysis.test.tsx`
- Modify: `web/src/connections/AdvancedSettings.test.tsx`
- Modify: `web/src/connections/HostDetail.test.tsx`
- Modify: `web/src/connections/ManageConnection.test.tsx`
- Modify: `web/src/connections/ConnectionsPage.test.tsx`

**Interfaces:**
- Produces exact English/Japanese copy for summary labels, three visible tabs, checks, dirty-state reasons, unavailable resource states, discard confirmation, management, and saved-but-refresh-failed state.
- Produces built-binary proof that no operation auto-starts and legacy URLs remain useful.

- [ ] **Step 1: Add both-locale message keys and run i18n validation**

Add every new key to both dictionaries in the same order. Use user-facing Japanese terms `基本`, `設定解析`, `詳細設定`, `到達性を確認`, `保存済み設定で認証を確認`, `未保存の変更あり`, and `保存済みですが、表示を更新できませんでした`. Do not expose internal terms such as `Loadable`, `dirty`, or `revision`.

```bash
npm test --prefix web -- --run src/i18n/i18n.test.tsx
```

- [ ] **Step 2: Update built-binary connection journeys**

Replace six-tab expectations with the three visible tabs. Add a request recorder and prove that opening a host and visiting `tab=diagnostics` starts no `/diagnostics/*` or `/terminal/launch` request. Assert summary and Basic fields are visible together, a draft disables actions, tab round-trip preserves the draft, save updates the summary, and changing host prompts before discard.

- [ ] **Step 3: Add route compatibility end-to-end assertions**

Directly visit each old URL and assert:

```text
tab=basic       -> Basic
tab=diagnostics -> Basic with checks expanded and no request
tab=effective   -> Settings analysis
tab=jump        -> Advanced settings / Jump
tab=advanced    -> Advanced settings / Directives
tab=raw         -> Advanced settings / Raw
```

- [ ] **Step 4: Run frontend verification**

```bash
npm test --prefix web
npm run typecheck --prefix web
npm run build --prefix web
npm run e2e --prefix web -- connections.spec.ts routing.spec.ts
```

Expected: unit tests, TypeScript, production build, and focused Playwright all pass. The known Vite chunk-size warning may be reported but is not a failure.

- [ ] **Step 5: Run repository and generated-output verification**

```bash
make verify-generated
go test ./...
go test -race ./...
git diff --check
```

No API generation diff is expected because this feature adds no server schema.

- [ ] **Step 6: Run Docker-backed integration**

```bash
make integration-up
make integration
make integration-down
```

Always run `make integration-down` after the integration attempt. Report Docker unavailable separately from a product-test failure.

- [ ] **Step 7: Commit the final integration layer**

```bash
git add web/src/i18n/messages.ts web/e2e/connections.spec.ts web/e2e/routing.spec.ts
git commit -m "test: cover connection summary workflow"
```

- [ ] **Step 8: Final inspection**

```bash
git status --short
git log --oneline -8
```

Expected: only intentional commits and no uncommitted source changes. Do not push until verification has completed.
