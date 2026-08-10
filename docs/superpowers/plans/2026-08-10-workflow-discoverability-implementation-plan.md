# Workflow Discoverability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make connection deep links, connection metadata, key next steps, group movement and staged group saving discoverable without changing backend transaction or secret boundaries.

**Architecture:** Extend the dependency-free section router with synchronized browser-location state, then keep connection query parsing in a focused pure module. Lift host tab state to `ConnectionsPage`, pass contextual labels through the existing inspector contract, and use short-lived shell state only for generated key IDs/relative paths. Existing mutation APIs remain authoritative and every suggested action remains an unsaved or unexecuted draft until its current explicit confirmation.

**Tech Stack:** React 19, TypeScript, Vitest, Testing Library, Playwright, existing Go/Echo server and OpenAPI client.

## Global Constraints

- Do not install packages or change dependency manifests.
- Never put passwords, passphrases, private-key material, absolute paths or form secrets in URL/history/shell workflow state.
- Do not contact a remote host until the existing explicit plan/check/connect/register action.
- Use path plus alias for connection identity; alias alone is not unique.
- Preserve current backend mutation boundaries and encrypted-vault behavior.
- Add every visible string to both English and Japanese message catalogs.

---

### Task 1: Location-aware section and connection routes

**Files:**
- Create: `web/src/routing/connectionRoute.ts`
- Create: `web/src/routing/connectionRoute.test.ts`
- Modify: `web/src/routing/useSectionRoute.ts`
- Modify: `web/src/routing/useSectionRoute.test.tsx`

**Interfaces:**
- Produces: `HostEditorTab`, `ConnectionTarget`, `parseConnectionSearch(search)`, and `connectionLocation(target)`.
- Produces: `BrowserLocation { pathname: string; search: string }` and `navigateLocation(url, { replace? })` from `useSectionRoute`.

- [ ] **Step 1: Write route RED tests.** Assert canonical query ordering/encoding, duplicate-safe path+alias identity, allowed tab parsing, invalid-tab Basic fallback, partial identity rejection, and no absolute-path acceptance.
- [ ] **Step 2: Run `npm test --prefix web -- --run src/routing/connectionRoute.test.ts` and confirm the missing module/behavior fails.**
- [ ] **Step 3: Implement the pure route module.** Use `URLSearchParams`, lowercase tab slugs, `/connections` for null selection, and reject paths beginning `/`, `~`, or containing NUL/backslash traversal forms.
- [ ] **Step 4: Run the focused route test and confirm GREEN.**
- [ ] **Step 5: Add hook RED tests.** Assert `location` includes search, internal location navigation pushes/replaces and rerenders, popstate restores query state, and primary section navigation clears it.
- [ ] **Step 6: Run `npm test --prefix web -- --run src/routing/useSectionRoute.test.tsx` and confirm the old hook lacks the interface.**
- [ ] **Step 7: Refactor `useSectionRoute` around one `{pathname, search}` state and implement `navigateLocation`.** Keep trailing-slash canonicalization and bootstrap-hash exclusion unchanged.
- [ ] **Step 8: Run both routing tests and `npm run typecheck --prefix web`, then commit.**

### Task 2: Deep-linked connection selection and tabs

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.test.tsx`
- Modify: `web/src/connections/ConnectionsPage.tsx`
- Modify: `web/src/connections/ConnectionsPage.test.tsx`
- Modify: `web/src/connections/HostDetail.tsx`
- Modify: `web/src/connections/HostDetail.test.tsx`
- Modify: `web/src/i18n/messages.ts`

**Interfaces:**
- Consumes: Task 1 `BrowserLocation`, `ConnectionTarget`, `HostEditorTab`, `connectionLocation` and `parseConnectionSearch`.
- Produces: controlled `tab` and `onTabChange` props on `HostDetailPanel`.

- [ ] **Step 1: Add HostDetail RED tests.** Assert an externally supplied tab is selected and clicking a tab reports the requested `HostEditorTab` without owning hidden local route state.
- [ ] **Step 2: Run the focused HostDetail test and confirm RED.**
- [ ] **Step 3: Make HostDetail tabs controlled and move the tab reset responsibility to its parent.**
- [ ] **Step 4: Add Connections RED tests.** Assert initial query selection loads the exact path+alias, selection and tab changes call canonical browser navigation, Back/Forward props restore detail/tab, a stale deep link shows a specific recovery action, and deletion clears the query.
- [ ] **Step 5: Run the focused Connections test and confirm RED.**
- [ ] **Step 6: Pass location/navigation from App; synchronize selection/tab from search; replace URLs after create, rename, group/file move and delete.** Refactor `reload`/`submit` only enough to return the refreshed overview needed to locate a moved identity.
- [ ] **Step 7: Add App RED/GREEN coverage for same-section navigation clearing connection query state and for passing location changes into the mounted Connections page.**
- [ ] **Step 8: Run App, routing, Connections and HostDetail tests plus typecheck, then commit.**

### Task 3: Co-located connection organisation and contextual metadata

**Files:**
- Modify: `web/src/ui/Inspector.tsx`
- Modify: `web/src/ui/Inspector.test.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.test.tsx`
- Modify: `web/src/connections/HostDetail.tsx`
- Modify: `web/src/connections/HostDetail.test.tsx`
- Modify: `web/src/connections/HostInspector.tsx`
- Modify: `web/src/connections/HostInspector.test.tsx`
- Modify: `web/src/connections/ConnectionTree.tsx`
- Modify: `web/src/connections/ConnectionTree.test.tsx`
- Modify: `web/src/connections/ConnectionsPage.tsx`
- Modify: `web/src/connections/ConnectionsPage.test.tsx`
- Modify: `web/src/groups/GroupsPanel.tsx`
- Modify: `web/src/groups/GroupsPanel.test.tsx`
- Modify: `web/src/groups/GroupInspector.tsx`
- Modify: `web/src/i18n/messages.ts`

**Interfaces:**
- Changes: `InspectorContent` becomes `{ label: string; attention: boolean; body: ReactNode } | null`.
- Produces: contextual inspector toggle/pane labels and explicit persistence hints.

- [ ] **Step 1: Add inspector RED tests.** Assert the visible toggle and pane use the supplied contextual label, not generic Details.
- [ ] **Step 2: Run the focused inspector/App tests and confirm RED.**
- [ ] **Step 3: Extend the inspector contract and supply `Display and classification` for hosts and `Group display settings` for groups.** Add immediate-save and staged-save copy inside the respective panes.
- [ ] **Step 4: Add HostDetail RED tests.** Assert Organisation appears in Basic, disappears from every other tab, and explains its independent save actions.
- [ ] **Step 5: Run HostDetail tests, implement the conditional placement, and confirm GREEN.**
- [ ] **Step 6: Add tree/connection RED tests.** Assert grouped connection rows show a visible drag handle/instruction and the file disclosure says `Change storage file` with a group-vs-file explanation.
- [ ] **Step 7: Implement the discoverability copy/affordances while retaining the existing drag payload and move API.**
- [ ] **Step 8: Run all affected component tests and typecheck, then commit.**

### Task 4: Generated-key next steps

**Files:**
- Modify: `web/src/keys/KeysScreen.tsx`
- Modify: `web/src/keys/KeysScreen.test.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.test.tsx`
- Modify: `web/src/connections/ConnectionsPage.tsx`
- Modify: `web/src/connections/ConnectionsPage.test.tsx`
- Modify: `web/src/connections/ConnectionBasicForm.tsx`
- Modify: `web/src/connections/ConnectionBasicForm.test.tsx`
- Modify: `web/src/remotekeys/RemoteKeyPanel.tsx`
- Modify: `web/src/remotekeys/RemoteKeyPanel.test.tsx`
- Modify: `web/src/i18n/messages.ts`

**Interfaces:**
- Produces: `GeneratedKeyNextStep { privateKeyId; privateRelativePath; publicRelativePath }` callback from Keys.
- Produces: optional suggested-key props for Connections/Basic and optional initial public relative path for RemoteKeyPanel.

- [ ] **Step 1: Add Keys RED tests.** Assert the header contains `href="#create-key-heading"`, successful in-process generation shows identified next steps, callbacks carry IDs/relative paths only, and hardware command generation shows no nonexistent-key actions.
- [ ] **Step 2: Run the focused Keys tests and confirm RED.**
- [ ] **Step 3: Capture the existing generate response, render the next-steps card and expose typed callbacks.**
- [ ] **Step 4: Add App/Connections/Basic RED tests.** Assert assign handoff selects Connections, prompts for a connection, preselects only a freshly inventoried matching private-key ID as a dirty draft, requires existing Save Basic settings, and clears after matching save/dismissal.
- [ ] **Step 5: Run those focused tests and confirm RED, then implement transient non-secret shell state and safe preselection.**
- [ ] **Step 6: Add RemoteKey RED tests.** Assert a supplied public relative path is resolved against the new inventory, preloads path/key text exactly once, and contacts no remote planning/register endpoint.
- [ ] **Step 7: Implement remote-key handoff and clear shell suggestion after successful preload/dismissal.**
- [ ] **Step 8: Run affected tests and typecheck, then commit.**

### Task 5: Group creation and sticky staged-save bar

**Files:**
- Modify: `web/src/groups/GroupsPanel.tsx`
- Modify: `web/src/groups/GroupsPanel.test.tsx`
- Modify: `web/src/i18n/messages.ts`

**Interfaces:**
- Consumes: existing `unsaved`, `run("preview")`, and `run("save")` behavior.
- Produces: top Add group card and `role="region"` sticky staged-change actions.

- [ ] **Step 1: Add Groups RED tests.** Assert Add group precedes the list, clean state has no sticky action bar, staged changes reveal the sticky bar, and its copy distinguishes staged Save from immediate rename/remove.
- [ ] **Step 2: Run the focused Groups test and confirm RED.**
- [ ] **Step 3: Move the existing creation card without duplicating state, render the sticky action bar only when dirty, and retain preview/save handlers.**
- [ ] **Step 4: Run Groups tests and typecheck, then commit.**

### Task 6: Browser flows, generated assets and final verification

**Files:**
- Modify: `web/e2e/routing.spec.ts`
- Modify: `web/e2e/connections.spec.ts`
- Modify: `web/e2e/keys.spec.ts`
- Rebuild: `internal/ui/dist/`

**Interfaces:**
- Consumes: Tasks 1-5 complete behavior.
- Produces: shipped embedded frontend and end-to-end regression evidence.

- [ ] **Step 1: Add E2E RED cases.** Cover direct host/tab URL, selection/tab Back and Forward, URL replacement after rename, top key-create action and generated-key assignment/installation handoffs.
- [ ] **Step 2: Run focused Playwright against the previously embedded bundle and confirm the new expectations fail for the intended missing behavior.**
- [ ] **Step 3: Rebuild the embedded frontend with the repository's existing build target and rerun focused E2E to GREEN.**
- [ ] **Step 4: Run `make verify-generated`, `make test`, `make e2e`, `make integration-up`, `make integration`, and `make integration-down`; confirm all exit zero and containers/config are removed.**
- [ ] **Step 5: Run `git diff --check`, inspect `git status --short`, inspect dependency-manifest changes, and request an independent code review.**
- [ ] **Step 6: Fix every valid Critical/Important review finding with its own RED/GREEN regression cycle, rerun the full relevant boundary, and commit generated assets.**
- [ ] **Step 7: Push `main`, watch the resulting GitHub Actions run to completion, and verify local HEAD equals `origin/main`.**
