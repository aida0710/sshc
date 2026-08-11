# Connection Browser Drilldown Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Connections file/group arrangement tree with a server-first browser and URL-addressable group drilldown while preserving connection drafts, safe identity selection, and explicit operations.

**Architecture:** A strict connection-location parser owns the new canonical server/group URLs and direct three-panel vocabulary. A pure overview index joins declared `Overview.groups` with group display metadata once, then supplies one-level group projections and recursive search to a new `ConnectionBrowser`. `ConnectionsPage` continues to own selected detail and mutations, but treats browser position and connection identity as independent URL state.

**Tech Stack:** React 19, TypeScript 5.9, Vite 8, Tailwind CSS 4, Vitest, Testing Library, Playwright, Go 1.25, Docker-backed SeaweedFS and OpenSSH integration.

## Global Constraints

- Add no package and change no dependency version.
- Default Connections to `/connections/servers`; `/connections` replaces itself with that URL.
- Preserve no legacy connection-query or six-tab URL compatibility.
- Keep concrete connection identity duplicate-safe with both config `path` and `host` alias.
- Keep drafts, secrets, search text, favourite filtering, and check results out of URL, history state, and persistent browser storage.
- Browser-position changes for the same identity must preserve drafts and make no detail API request.
- Only another identity triggers the existing discard confirmation.
- Never start Connect, reachability, authentication, `ssh -G`, or Terminal merely by opening or traversing the browser.
- Use declared `Overview.groups` as the group vocabulary; use `metadata.groups` only for display metadata such as hidden/order.
- Do not display pattern-only SSH rules as servers; the Config section remains their editor.
- Disable config/group drag moves while an editor is dirty or a committed snapshot is being reconfirmed.
- Keep the desktop two-pane layout and do not add a smartphone-specific layout.
- Use `apply_patch` for edits and preserve unrelated user changes.

---

## File map

- Modify `web/src/routing/sectionRoute.ts` and test: recognize every `/connections/...` URL as the Connections section without accepting other section descendants.
- Rewrite `web/src/routing/connectionRoute.ts` and test: canonical browser, identity, panel, invalid, and redirect states.
- Create `web/src/connections/connectionBrowser.ts` and test: one overview index, group levels, recursive search, hidden-container promotion, and stable ordering.
- Create `web/src/connections/ConnectionBrowser.tsx` and test: server/group toggle, flat servers, group drilldown, breadcrumbs, filtering, empty states, and visible-target drag/drop.
- Delete `web/src/connections/ConnectionTree.tsx` and its test after `ConnectionsPage` switches to the browser.
- Modify `web/src/connections/HostDetail.tsx`, `AdvancedSettings.tsx`, and tests: use direct three-panel and advanced-area route types instead of legacy tabs.
- Modify `web/src/connections/ConnectionsPage.tsx` and tests: independent browser/identity URL state, canonical redirect, missing/invalid route states, mutation handoff, and draft guard.
- Modify `web/src/App.tsx`, `App.test.tsx`, and `web/src/i18n/messages.ts`: remove pattern-rule callback from Connections and add exact browser copy in both locales.
- Modify `web/e2e/connections.spec.ts`, `groups.spec.ts`, and `routing.spec.ts`: canonical browser journeys and removal of old URL assumptions.
- Refresh `internal/ui/dist` only after all source verification passes.

---

### Task 1: Introduce the canonical connection location model

**Files:**
- Modify: `web/src/routing/sectionRoute.ts`
- Modify: `web/src/routing/sectionRoute.test.ts`
- Modify: `web/src/routing/connectionRoute.ts`
- Modify: `web/src/routing/connectionRoute.test.ts`

**Interfaces:**
- Produces `ConnectionPanel = "Basic" | "Analysis" | "Advanced"`.
- Produces `AdvancedArea = "Jump" | "Directives" | "Raw"`.
- Produces `ConnectionBrowserLocation`, `ConnectionTarget`, and `ParsedConnectionLocation`.
- Produces `parseConnectionLocation({ pathname, search })` and `connectionLocation(browser, target)`.
- Keeps the old route exports temporarily until Task 4 switches all consumers; Task 4 removes them and their tests.

- [ ] **Step 1: Write failing section-prefix tests**

Add assertions that connection descendants remain inside the Connections section while another section's descendants remain not found:

```ts
expect(parseSectionPath("/connections/servers")).toMatchObject({
  kind: "section", section: "Connections", canonical: true,
});
expect(parseSectionPath("/connections/groups/home/eu")).toMatchObject({
  kind: "section", section: "Connections", canonical: true,
});
expect(parseSectionPath("/connections/child")).toMatchObject({
  kind: "section", section: "Connections", canonical: true,
});
expect(parseSectionPath("/connections//")).toMatchObject({
  kind: "section", section: "Connections", canonical: true,
});
expect(parseSectionPath("/keys/child")).toEqual({ kind: "not-found", pathname: "/keys/child" });
```

- [ ] **Step 2: Run the section test and verify RED**

Run:

```bash
npm test --prefix web -- --run src/routing/sectionRoute.test.ts
```

Expected: the two Connections descendant assertions fail as `not-found`.

- [ ] **Step 3: Recognize the Connections prefix without validating its inner route**

After the exact section loop, return Connections for a path beginning `/connections/`. Give the returned route the actual pathname as `canonicalPath`; connection-specific validation belongs to `connectionRoute.ts`.

```ts
if (pathname.startsWith("/connections/")) {
  return {
    kind: "section",
    section: "Connections",
    canonicalPath: pathname,
    canonical: true,
  };
}
```

Keep `/connections` as the existing exact section route so `ConnectionsPage` can replace it with the server default and deliberately discard old query state. Remove `/connections/child` and `/connections//` from the old global-not-found table: inner connection validation now owns both and will render the connection-specific invalid URL state.

- [ ] **Step 4: Write failing canonical route tests**

Define fixtures with these exact shapes:

```ts
const servers = { view: "servers" } as const;
const group = { view: "groups", scope: "named", group: "home/eu" } as const;
const target = {
  path: "connections/home/eu/app.conf",
  alias: "app prod",
  panel: "Advanced",
  advanced: "Raw",
} as const;

expect(parseConnectionLocation({ pathname: "/connections", search: "?tab=raw" }))
  .toEqual({ kind: "redirect", location: "/connections/servers" });
expect(connectionLocation(servers, null)).toBe("/connections/servers");
expect(connectionLocation(group, target)).toBe(
  "/connections/groups/home/eu?path=connections%2Fhome%2Feu%2Fapp.conf&host=app+prod&panel=advanced&advanced=raw",
);
expect(parseConnectionLocation({
  pathname: "/connections/groups/home/eu",
  search: "?path=connections%2Fhome%2Feu%2Fapp.conf&host=app+prod&panel=advanced&advanced=raw",
})).toEqual({ kind: "valid", browser: group, target });
```

Add invalid cases for partial identity, `tab=`, unknown panel, `advanced` without `panel=advanced`, a named group plus `scope=ungrouped`, duplicate keys, dot segments, encoded slash/backslash, control characters, and `/connections/files`.

- [ ] **Step 5: Run the route test and verify RED**

```bash
npm test --prefix web -- --run src/routing/connectionRoute.test.ts
```

Expected: compile failure because the new types and functions do not exist.

- [ ] **Step 6: Implement the strict parser and formatter**

Use these public types:

```ts
export type ConnectionPanel = "Basic" | "Analysis" | "Advanced";
export type AdvancedArea = "Jump" | "Directives" | "Raw";

export type ConnectionBrowserLocation =
  | { view: "servers" }
  | { view: "groups"; scope: "root" }
  | { view: "groups"; scope: "named"; group: string }
  | { view: "groups"; scope: "ungrouped" };

export type ConnectionTarget = {
  path: string;
  alias: string;
  panel: ConnectionPanel;
  advanced: AdvancedArea;
};

export type ParsedConnectionLocation =
  | { kind: "redirect"; location: "/connections/servers" }
  | { kind: "invalid" }
  | { kind: "valid"; browser: ConnectionBrowserLocation; target: ConnectionTarget | null };
```

Accept only `scope`, `path`, `host`, `panel`, and `advanced`, each at most once. A target requires both `path` and `host`, and always formats `panel`. Omit `advanced` unless panel is Advanced. With no target, reject panel fields. Encode each named-group segment independently so `/` remains hierarchy rather than data inside one segment.

- [ ] **Step 7: Run route suites and commit**

```bash
npm test --prefix web -- --run src/routing/sectionRoute.test.ts src/routing/connectionRoute.test.ts src/routing/useSectionRoute.test.tsx
git add web/src/routing/sectionRoute.ts web/src/routing/sectionRoute.test.ts web/src/routing/connectionRoute.ts web/src/routing/connectionRoute.test.ts
git commit -m "feat: define canonical connection browser routes"
```

---

### Task 2: Build one declared-group browser index

**Files:**
- Create: `web/src/connections/connectionBrowser.ts`
- Create: `web/src/connections/connectionBrowser.test.ts`

**Interfaces:**
- Consumes `Overview`, `HostEntry`, and `ConnectionBrowserLocation`.
- Produces `BrowserServer`, `BrowserGroup`, `ConnectionBrowserIndex`, `BrowserProjection`.
- Produces `buildConnectionBrowserIndex(overview)` and `projectConnectionBrowser(index, browser, query, favouritesOnly)`.

- [ ] **Step 1: Write failing server-index tests**

Create an overview with two duplicate `nas` aliases in different paths, one pattern-only entry with an empty alias, metadata order/colour/tags, declared groups `home`, `home/eu`, `work`, and an ungrouped server. Assert:

```ts
const index = buildConnectionBrowserIndex(overview);
expect(index.servers.map((server) => [server.identity.path, server.identity.alias])).toEqual([
  ["connections/home/nas.conf", "nas"],
  ["connections/work/nas.conf", "nas"],
  ["config", "bastion"],
]);
expect(index.servers.some((server) => server.host.identity.alias === "")).toBe(false);
expect(index.duplicateAliases.has("nas")).toBe(true);
```

The expected order must follow metadata order and keep overview order for ties.

- [ ] **Step 2: Write failing group-level tests**

Assert the root returns top-level declared groups and ungrouped count; `home` returns `home/eu` plus only direct `home` servers; `home/eu` returns its own direct servers. Empty declared `work` must remain visible.

```ts
expect(projectConnectionBrowser(index, { view: "groups", scope: "root" }, "", false))
  .toMatchObject({ kind: "group-level", groups: [
    { name: "home", descendantCount: 2 },
    { name: "work", descendantCount: 1 },
  ] });
expect(projectConnectionBrowser(index, { view: "groups", scope: "named", group: "home" }, "", false))
  .toMatchObject({ kind: "group-level", groups: [{ name: "home/eu" }], servers: [
    { identity: { alias: "nas" } },
  ] });
```

Add hidden-container cases: an empty hidden container promotes visible descendants; a hidden group with a direct server stays visible. Add an undeclared `host.group` and prove no group is invented.

- [ ] **Step 3: Write failing search/favourite tests**

From `home`, query a descendant alias and assert a flat `search-results` projection with full group path. Query a group path and assert its servers match. With favourites-only, keep direct favourites and groups with favourite descendants; distinguish a zero-match filter from an empty data set.

- [ ] **Step 4: Run the pure suite and verify RED**

```bash
npm test --prefix web -- --run src/connections/connectionBrowser.test.ts
```

Expected: module-not-found failure.

- [ ] **Step 5: Implement the index and projections**

Use serializable arrays plus internal maps:

```ts
export type BrowserServer = {
  host: HostEntry;
  identity: HostEntry["identity"];
  group: string;
  tags: string[];
  favourite: boolean;
  colour: string;
  order: number;
  duplicateAlias: boolean;
};

export type BrowserGroup = {
  name: string;
  label: string;
  parent: string;
  hidden: boolean;
  order: number;
  descendantCount: number;
  favouriteDescendantCount: number;
};
```

The declared vocabulary comes from `overview.groups`. Join metadata by name for hidden/order and host metadata by `path + NUL + alias`. Derive the nearest declared ancestor from slash-separated names rather than inventing missing path components. Precompute direct servers, descendants, counts, and promoted visible children when overview changes.

Return one of:

```ts
export type BrowserProjection =
  | { kind: "servers"; servers: BrowserServer[] }
  | { kind: "group-level"; group: string | null; groups: BrowserGroup[]; servers: BrowserServer[]; ungroupedCount: number }
  | { kind: "search-results"; scope: string | null; servers: BrowserServer[] }
  | { kind: "missing-group"; group: string };
```

- [ ] **Step 6: Run the pure suite and commit**

```bash
npm test --prefix web -- --run src/connections/connectionBrowser.test.ts
git add web/src/connections/connectionBrowser.ts web/src/connections/connectionBrowser.test.ts
git commit -m "feat: index connection browser groups"
```

---

### Task 3: Render the server-first connection browser

**Files:**
- Create: `web/src/connections/ConnectionBrowser.tsx`
- Create: `web/src/connections/ConnectionBrowser.test.tsx`
- Modify: `web/src/i18n/messages.ts`

**Interfaces:**
- Consumes `ConnectionBrowserLocation`, `ConnectionBrowserIndex`, and `DragPayload`.
- Produces `ConnectionBrowser` with browser, selection, browse/select/drop callbacks, and `movesDisabled`.
- Leaves the old `ConnectionTree` in place until Task 4 switches the page.

- [ ] **Step 1: Write failing default-server tests**

Render the new component at `{ view: "servers" }`. Assert the Server segment is pressed, concrete servers are flat, group labels are secondary text, duplicate aliases show source paths, pattern rules do not render, search spans every group, and favourites preserve metadata order.

```ts
expect(screen.getByRole("button", { name: "Servers" })).toHaveAttribute("aria-pressed", "true");
expect(screen.getAllByRole("button", { name: /nas/ })).toHaveLength(2);
expect(screen.queryByText(/Host \*/)).not.toBeInTheDocument();
expect(screen.getByText("home/eu", { exact: true })).toBeInTheDocument();
```

- [ ] **Step 2: Write failing drilldown and empty-state tests**

Render group root, named `home`, nested `home/eu`, ungrouped, and a missing group. Assert root has only top groups/ungrouped, named groups show breadcrumb + children + direct servers, empty groups say servers none, and missing groups provide a callback to group root.

Verify clicking a group calls:

```ts
expect(onBrowse).toHaveBeenCalledWith({ view: "groups", scope: "named", group: "home/eu" });
```

Verify search in `home` produces descendant server rows with full group paths rather than child cards.

- [ ] **Step 3: Write failing visible-target drag tests**

Test direct server to child group, direct server to parent breadcrumb, child group to sibling, and child group to parent breadcrumb. Assert Server mode, dirty/refresh disabled mode, self drop, and descendant drop call nothing. The parent destination is the parent group name, or empty for ungrouped/top-level.

- [ ] **Step 4: Run the component test and verify RED**

```bash
npm test --prefix web -- --run src/connections/ConnectionBrowser.test.tsx
```

Expected: module-not-found failure.

- [ ] **Step 5: Implement the browser without data fetching**

Use this prop contract:

```ts
type ConnectionBrowserProps = {
  overview: Overview;
  browser: ConnectionBrowserLocation;
  selected: HostSelection | null;
  movesDisabled: boolean;
  onBrowse: (browser: ConnectionBrowserLocation) => void;
  onSelect: (host: HostEntry) => void;
  onDrop: (payload: DragPayload, target: string) => void;
};
```

Build the index with `useMemo([overview])`. Keep query and favourites-only local. Use `Segmented` with Servers first. Render group rows as buttons with count text, breadcrumb items as browser callbacks, and concrete server rows through one shared renderer. Do not use an effect to navigate or start an operation.

Add exact English/Japanese messages in matching order for browser mode, server/group labels, group counts, breadcrumbs, ungrouped, current-scope filter, no matches, empty server data, empty group data, missing group, invalid URL, and recovery actions.

- [ ] **Step 6: Run browser and i18n suites and commit**

```bash
npm test --prefix web -- --run src/connections/ConnectionBrowser.test.tsx src/connections/connectionBrowser.test.ts src/i18n/i18n.test.tsx
git add web/src/connections/ConnectionBrowser.tsx web/src/connections/ConnectionBrowser.test.tsx web/src/i18n/messages.ts
git commit -m "feat: browse servers and nested groups"
```

---

### Task 4: Integrate browser location with connection detail and drafts

**Files:**
- Modify: `web/src/connections/HostDetail.tsx`
- Modify: `web/src/connections/HostDetail.test.tsx`
- Modify: `web/src/connections/AdvancedSettings.tsx`
- Modify: `web/src/connections/AdvancedSettings.test.tsx`
- Modify: `web/src/connections/ConnectionsPage.tsx`
- Modify: `web/src/connections/ConnectionsPage.test.tsx`
- Delete: `web/src/connections/ConnectionTree.tsx`
- Delete: `web/src/connections/ConnectionTree.test.tsx`
- Modify: `web/src/routing/connectionRoute.ts`
- Modify: `web/src/routing/connectionRoute.test.ts`

**Interfaces:**
- Consumes the canonical route and `ConnectionBrowser` from Tasks 1–3.
- `HostDetailPanel` consumes `panel`, `advanced`, and `onLocationChange(panel, advanced)` directly.
- `ConnectionsPage` treats browser position and target identity as independent route state.
- Removes every legacy tab type, mapper, and compatibility assertion.

- [ ] **Step 1: Write failing direct-panel tests**

Replace `HostEditorTab` tests with direct props:

```tsx
<HostDetailPanel panel="Advanced" advanced="Raw" onLocationChange={onLocationChange} ... />
```

Assert Basic, Analysis, and Advanced select the matching visible area; Jump/Directives/Raw select advanced areas; no diagnostics URL state exists; mounted Basic and Advanced drafts still persist across visible panel changes.

- [ ] **Step 2: Convert HostDetail and AdvancedSettings to direct route types**

Remove `connectionAreaForTab` and `tabForConnectionArea`. `HostDetailPanel` uses:

```ts
panel?: ConnectionPanel;
advanced?: AdvancedArea;
onLocationChange?: (panel: ConnectionPanel, advanced: AdvancedArea) => void;
```

Keep `lastAdvanced` for an uncontrolled Basic/Analysis round-trip, but controlled navigation publishes canonical panel/advanced pairs. All three editor components remain mounted under `hidden` wrappers.

- [ ] **Step 3: Write failing page URL-state tests**

Cover these behaviors in `ConnectionsPage.test.tsx`:

1. `/connections` calls replace with `/connections/servers` and carries no old query.
2. Server root renders all concrete connections without calling `configApi.host`.
3. Toggle/group/breadcrumb navigation changes only browser path and does not refetch the selected detail.
4. Selecting a host adds `path`, `host`, `panel=basic` to the current browser URL.
5. Panel/advanced changes keep browser path.
6. Same-identity browser navigation is allowed while dirty and preserves the draft.
7. Another identity still prompts; rejection preserves current URL/identity/draft.
8. Missing group and invalid route show distinct recovery actions.
9. Rename/move/create follow committed identity in the current browser; delete removes only the target query.

- [ ] **Step 4: Implement page route orchestration**

Parse from both `location.pathname` and `location.search`. Keep `browser` derived from the valid route, and keep local selection/detail only as the loaded async state for its target.

Use helpers with explicit browser arguments:

```ts
function navigateBrowser(next: ConnectionBrowserLocation, options?: NavigateLocationOptions): boolean;
function navigateTarget(identity: HostSelection, panel: ConnectionPanel, advanced: AdvancedArea, options?: NavigateLocationOptions): boolean;
function clearTarget(options?: NavigateLocationOptions): boolean;
```

`navigateBrowser` formats the next browser with the current target unchanged. The navigation blocker compares only target `path + alias`; it allows browser and panel changes for the same identity. An invalid next connection route is a leave operation and prompts while dirty.

Render `ConnectionBrowser` in the left pane. A missing group is a browser state; an invalid URL shows the invalid-location recovery state and does not guess a selection. The overview remains available for recovery actions.

When a selected identity changes, clear the previous detail before fetching. When only browser/panel changes, keep detail and saved state intact. Guard stale async replies with the existing active flag and `selectionRef`.

- [ ] **Step 5: Remove the old tree and legacy route surface**

Delete `ConnectionTree.tsx` and its test only after the page imports `ConnectionBrowser`. Remove from `connectionRoute.ts`:

```text
hostEditorTabs
HostEditorTab
connectionAreaForTab
checksExpandedForTab
tabForConnectionArea
parseConnectionSearch
```

Remove all legacy tests instead of translating them into compatibility tests.

- [ ] **Step 6: Run focused integration suites and commit**

```bash
npm test --prefix web -- --run src/routing/connectionRoute.test.ts src/connections/HostDetail.test.tsx src/connections/AdvancedSettings.test.tsx src/connections/ConnectionBrowser.test.tsx src/connections/ConnectionsPage.test.tsx
npm run typecheck --prefix web
git add web/src/routing/connectionRoute.ts web/src/routing/connectionRoute.test.ts web/src/connections/HostDetail.tsx web/src/connections/HostDetail.test.tsx web/src/connections/AdvancedSettings.tsx web/src/connections/AdvancedSettings.test.tsx web/src/connections/ConnectionsPage.tsx web/src/connections/ConnectionsPage.test.tsx web/src/connections/ConnectionBrowser.tsx web/src/connections/ConnectionBrowser.test.tsx web/src/connections/ConnectionTree.tsx web/src/connections/ConnectionTree.test.tsx
git commit -m "feat: integrate connection browser routes"
```

---

### Task 5: Update shell and built-binary journeys

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.test.tsx`
- Modify: `web/e2e/connections.spec.ts`
- Modify: `web/e2e/groups.spec.ts`
- Modify: `web/e2e/routing.spec.ts`

**Interfaces:**
- Connections no longer consumes `onOpenFile`; Config remains the only pattern/file editor.
- Primary Connections link still points to `/connections`, which the page replaces with `/connections/servers`.
- Playwright contracts only the new canonical URLs.

- [ ] **Step 1: Write failing shell tests for the new entry URL**

Update the Connections mock to remove `onOpenFile`, and assert choosing Connections settles at `/connections/servers`. Assert nested group URLs keep the Connections nav item current and render the Connections section rather than global Not Found.

- [ ] **Step 2: Remove the obsolete Connections pattern callback**

Delete `onOpenFile` from `ConnectionsPageProps`, the App mock, and the Connections `SectionView` invocation. Keep `SectionViewProps.onOpenFile` for Config and other padded sections. Change only the Connections branch.

- [ ] **Step 3: Replace old routing E2E with canonical journeys**

`routing.spec.ts` must assert:

```text
/connections -> /connections/servers
/connections/servers + selected host -> panel=basic
/connections/groups -> top groups
/connections/groups/home -> child groups + direct servers
/connections/groups/home/eu -> nested level
Back/Forward/reload -> same browser + target + panel
```

Delete the six legacy `tab=` cases. Record POST requests and prove browser traversal starts no diagnostics or terminal operation.

- [ ] **Step 4: Update connection/group E2E selectors**

`openBastion` starts from the default Servers browser. Group-move tests explicitly switch to Groups before dragging. Management-based group changes remain unchanged. Add a journey that edits Basic, switches Servers → Groups → nested group while retaining the same draft, then rejects and accepts a different server selection.

- [ ] **Step 5: Run all frontend verification and commit**

```bash
npm test --prefix web
npm run typecheck --prefix web
npm run build --prefix web
npm run e2e --prefix web
git diff --check
git add web/src/App.tsx web/src/App.test.tsx web/e2e/connections.spec.ts web/e2e/groups.spec.ts web/e2e/routing.spec.ts web/src/i18n/messages.ts
git commit -m "test: cover connection browser drilldown"
```

Expected: every Web unit and Playwright test passes; the platform-specific Linux terminal test may skip on macOS. The known Vite chunk-size warning may remain, but build exit must be zero.

---

### Task 6: Verify repository, Docker integration, built assets, and push

**Files:**
- Modify generated build output under `internal/ui/dist` only through `npm run build --prefix web`.

**Interfaces:**
- No API schema change is expected.
- Produces the pushed `main` containing source, tests, design, plan, and embedded Web assets.

- [ ] **Step 1: Verify generated API and repository suites**

```bash
make verify-generated
go test ./...
go test -race ./...
git diff --check
```

Expected: generated API has no diff; Go normal/race and whitespace checks pass.

- [ ] **Step 2: Run Docker-backed integration with guaranteed cleanup**

```bash
set -e
trap 'make integration-down' EXIT
make integration-up
make integration
```

Expected: real SeaweedFS conditional-write/encryption tests and OpenSSH askpass authentication tests pass; `sshc-s3`, `sshc-sshd`, and `.integration-s3.json` are absent afterward.

- [ ] **Step 3: Rebuild and commit embedded assets**

```bash
npm run build --prefix web
git add internal/ui/dist
git commit -m "build: refresh connection browser assets"
```

- [ ] **Step 4: Perform final evidence-based inspection**

```bash
git status --short
git log --oneline -12
git diff --check
git fetch origin main
git rev-list --left-right --count origin/main...main
```

Expected: clean tree; zero remote-only commits. If remote-only commits exist, stop instead of rebasing or force-pushing without review.

- [ ] **Step 5: Push without force and verify the remote head**

```bash
git push origin main
test "$(git rev-parse HEAD)" = "$(git ls-remote origin refs/heads/main | awk '{print $1}')"
git status -sb
```

Expected: local and remote hashes match and `main` has no ahead/behind marker.
