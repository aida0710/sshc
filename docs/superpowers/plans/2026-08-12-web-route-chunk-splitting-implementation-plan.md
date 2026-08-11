# Web Route Chunk Splitting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split non-home Web sections into on-demand Vite chunks so the initial JavaScript falls by at least 30% to below 400 kB and the 500 kB build warning disappears without adding a visible loading state.

**Architecture:** `App.tsx` keeps the shell, bootstrap, lock screen, and Home screen as eager imports. Every other top-level section becomes a `React.lazy` named-export adapter, and the existing `SectionView` is rendered inside `Suspense fallback={null}` so route and prop ownership remain unchanged. Vite owns shared-chunk extraction; no manual chunk configuration or preload path is added.

**Tech Stack:** React 19, TypeScript 5.9, Vite 8, Vitest, Testing Library, Playwright, Go 1.25, Docker-backed SeaweedFS and OpenSSH integration.

## Global Constraints

- Add no package and change no dependency version.
- Keep `LockScreen`, `OverviewPanel`, the shell, routing, theme, locale, and bootstrap in the eager graph.
- Lazy-load Connections, Config, Groups, Keys, Known Hosts, Remote Keys, Diagnostics, Secrets, Settings, Sync, and History by top-level section.
- Use `Suspense fallback={null}`; add no spinner, skeleton, loading copy, or ARIA loading announcement.
- Add no hover, focus, idle, or eager preload.
- Do not change `vite.config.ts`, `chunkSizeWarningLimit`, or manual chunk configuration.
- Preserve every existing route, API call, workflow handoff, inspector interaction, and unsaved-change boundary.
- Keep the built Web UI embedded in `internal/ui/dist` and keep all asset references hash-addressed.
- Push only after Web, Go normal/race, generated API, full Playwright, and Docker integration verification pass.

---

## File map

- Modify `web/src/App.tsx`: replace static section imports with named-export `lazy` adapters and add the invisible `Suspense` boundary.
- Modify `web/src/App.test.tsx` only where a lazy section now requires an async Testing Library query; add one explicit assertion that the shell remains available and no loading UI is exposed while a direct lazy route resolves.
- Refresh `internal/ui/dist/index.html` and hashed files only after source verification succeeds.
- Do not modify `web/vite.config.ts`, `web/package.json`, or either lockfile.

---

### Task 1: Split non-home section modules at the App boundary

**Files:**
- Modify: `web/src/App.tsx:1-37, 428-458`
- Modify: `web/src/App.test.tsx`

**Interfaces:**
- Consumes: existing named exports `ConnectionsPage`, `ConfigExplorer`, `GroupsPanel`, `HistoryPanel`, `KeysScreen`, `DiagnosticsPanel`, `SecretsPanel`, `SettingsPanel`, `SyncPanel`, `KnownHostsPanel`, and `RemoteKeyPanel`.
- Produces: module-scope lazy components with those same local names and unchanged prop contracts.
- Preserves: eager `LockScreen`, `OverviewPanel`, `UpdateBadge`, `SectionViewProps`, and every `SectionView` branch.

- [ ] **Step 1: Verify the current bundle budget is RED**

Run from the repository root:

```bash
bundle_output=$(npm run build --prefix web 2>&1)
printf '%s\n' "$bundle_output"
if [[ "$bundle_output" == *"Some chunks are larger than 500 kB"* ]]; then
  exit 1
fi
```

Expected: exit 1 after reporting the existing 569.91 kB entry chunk and the Vite warning.

- [ ] **Step 2: Add a shell-without-loading-UI regression test**

Add this case to `web/src/App.test.tsx` near the direct-section routing cases. It uses the already mocked `KeysScreen`, existing providers, `csrfToken`, and `openVault`:

```tsx
it("keeps the shell visible while a direct section module resolves without loading copy", async () => {
  window.history.replaceState(null, "", "/keys");
  render(
    <App
      bootstrap={vi.fn().mockResolvedValue({ csrfToken })}
      health={vi.fn().mockResolvedValue({ status: "ok", version: "0.1.0" })}
      vault={openVault}
    />,
  );

  expect(await screen.findByRole("link", { name: "Keys" })).toHaveAttribute(
    "aria-current",
    "page",
  );
  expect(screen.queryByText(/loading/i)).not.toBeInTheDocument();
  expect(await screen.findByText("keys panel")).toBeInTheDocument();
});
```

Run:

```bash
npm test --prefix web -- --run src/App.test.tsx
```

Expected before the refactor: PASS as a characterization of the required visible behavior. The bundle-budget command, not this UI behavior test, is the RED regression for the optimization.

- [ ] **Step 3: Replace static component imports with types and React lazy support**

Change the React import to:

```tsx
import { Suspense, lazy, useCallback, useEffect, useState, type MouseEvent } from "react";
```

Keep these section imports eager:

```tsx
import type { CreateConnectionDraft, CreationPrerequisite } from "./connections/CreateConnectionModal";
import type { FileTarget } from "./explorer/ConfigExplorer";
import { LockScreen } from "./secrets/LockScreen";
import { UpdateBadge } from "./shell/UpdateBadge";
import { OverviewPanel } from "./overview/OverviewPanel";
```

Delete the static value imports for the eleven delayed section components. Do not convert any API, UI primitive, route, inspector, workflow, or context import to a dynamic import.

- [ ] **Step 4: Declare one named-export lazy adapter per delayed section**

Place these module-scope declarations after the imports and before `AppProps`:

```tsx
const ConnectionsPage = lazy(() =>
  import("./connections/ConnectionsPage").then(({ ConnectionsPage }) => ({ default: ConnectionsPage })),
);
const ConfigExplorer = lazy(() =>
  import("./explorer/ConfigExplorer").then(({ ConfigExplorer }) => ({ default: ConfigExplorer })),
);
const GroupsPanel = lazy(() =>
  import("./groups/GroupsPanel").then(({ GroupsPanel }) => ({ default: GroupsPanel })),
);
const HistoryPanel = lazy(() =>
  import("./history/HistoryPanel").then(({ HistoryPanel }) => ({ default: HistoryPanel })),
);
const KeysScreen = lazy(() =>
  import("./keys/KeysScreen").then(({ KeysScreen }) => ({ default: KeysScreen })),
);
const DiagnosticsPanel = lazy(() =>
  import("./diagnostics/DiagnosticsPanel").then(({ DiagnosticsPanel }) => ({ default: DiagnosticsPanel })),
);
const SecretsPanel = lazy(() =>
  import("./secrets/SecretsPanel").then(({ SecretsPanel }) => ({ default: SecretsPanel })),
);
const SettingsPanel = lazy(() =>
  import("./settings/SettingsPanel").then(({ SettingsPanel }) => ({ default: SettingsPanel })),
);
const SyncPanel = lazy(() =>
  import("./sync/SyncPanel").then(({ SyncPanel }) => ({ default: SyncPanel })),
);
const KnownHostsPanel = lazy(() =>
  import("./knownhosts/KnownHostsPanel").then(({ KnownHostsPanel }) => ({ default: KnownHostsPanel })),
);
const RemoteKeyPanel = lazy(() =>
  import("./remotekeys/RemoteKeyPanel").then(({ RemoteKeyPanel }) => ({ default: RemoteKeyPanel })),
);
```

Do not add catch/retry wrappers or loader registries. Each declaration has one section and preserves TypeScript inference for its actual props.

- [ ] **Step 5: Add the invisible Suspense boundary around SectionView**

In the `route.kind === "section"` branch, wrap the existing `SectionView` call without moving or changing any prop:

```tsx
<Suspense fallback={null}>
  <SectionView
    section={route.section}
    fileTarget={fileTarget}
    groups={groups}
    knownAliases={knownAliases}
    connectionDraft={connectionDraft}
    onConnectionDraftChange={setConnectionDraft}
    onNavigateForCreation={(target: CreationPrerequisite) => navigate(target)}
    onOpenFile={openFile}
    onLock={() => setState("locked")}
    onInspector={setInspector}
    onNavigate={navigate}
    location={location}
    onNavigateLocation={navigateLocation}
    onNavigationBlockerChange={setNavigationBlocker}
    preferredConnectionKey={preferredConnectionKey}
    preferredPublicKey={preferredPublicKey}
    onAssignGeneratedKey={assignGeneratedKey}
    onInstallGeneratedKey={installGeneratedKey}
    onPreferredConnectionKeyApplied={consumePreferredConnectionKey}
    onPreferredPublicKeyHandled={consumePreferredPublicKey}
  />
</Suspense>
```

The not-found route remains outside this boundary because it has no delayed section module.

- [ ] **Step 6: Make only newly asynchronous test assertions await their screen**

Run the App suite:

```bash
npm test --prefix web -- --run src/App.test.tsx
```

If a post-navigation assertion races the dynamic import, change only that assertion from `getByText`/`getByRole` to the matching `findByText`/`findByRole`. Do not add timers, arbitrary waits, fake loading copy, or blanket `waitFor` wrappers.

Expected: every App test passes with the existing route, handoff, draft, inspector, lock, and history assertions intact.

- [ ] **Step 7: Verify types and the GREEN bundle budget**

Run:

```bash
npm run typecheck --prefix web
bundle_output=$(npm run build --prefix web 2>&1)
printf '%s\n' "$bundle_output"
if [[ "$bundle_output" == *"Some chunks are larger than 500 kB"* ]]; then
  exit 1
fi
entry_asset=$(sed -n 's/.*src="\/assets\/\([^"]*\.js\)".*/\1/p' internal/ui/dist/index.html)
entry_bytes=$(wc -c < "internal/ui/dist/assets/$entry_asset")
test "$entry_bytes" -lt 400000
```

Expected: typecheck succeeds, Vite emits multiple JavaScript chunks without the 500 kB warning, and the entry chunk is below 400,000 bytes (at least 30% below the 569,914-byte baseline).

- [ ] **Step 8: Confirm no forbidden configuration or dependency change**

Run:

```bash
git diff --exit-code -- web/vite.config.ts web/package.json web/package-lock.json package.json package-lock.json
git diff --check
```

Expected: both commands succeed.

- [ ] **Step 9: Commit the source change**

Do not stage `internal/ui/dist` yet. Run:

```bash
git add web/src/App.tsx web/src/App.test.tsx
git commit -m "perf: split web sections into lazy chunks"
```

---

### Task 2: Verify the shipped binary, refresh embedded assets, and publish

**Files:**
- Refresh: `internal/ui/dist/index.html`
- Replace: hashed files under `internal/ui/dist/assets/`

**Interfaces:**
- Consumes: the Task 1 dynamic-import graph and unchanged Make targets.
- Produces: a Go binary embedding the new entry and delayed section chunks.

- [ ] **Step 1: Run the complete repository test target**

Run:

```bash
make test
```

Expected: Go normal/race, all Vitest files, and both TypeScript projects pass.

- [ ] **Step 2: Verify generated API sources**

Run:

```bash
make verify-generated
```

Expected: regenerated Go and TypeScript API files have no diff.

- [ ] **Step 3: Run full Playwright against the built Go binary**

Run:

```bash
make e2e
```

Expected: all platform-applicable scenarios pass; the Linux-only terminal scenario may skip on macOS. The build phase emits no 500 kB warning, and the section appearance loop proves each delayed screen can load on first navigation.

- [ ] **Step 4: Run Docker-backed integration with guaranteed cleanup**

Confirm the fixed test names are not already present:

```bash
docker ps -a --filter name='^/sshc-s3$' --filter name='^/sshc-sshd$' --format '{{.Names}} {{.Status}} {{.Image}}'
```

Then run:

```bash
trap 'make integration-down' EXIT
make integration-up
make integration
```

Expected: real SeaweedFS conditional writes, encrypted sync, real OpenSSH askpass authentication, wrong-password, spent-token, and locked-vault boundaries pass. The EXIT trap removes only `sshc-s3`, `sshc-sshd`, and `.integration-s3.json`.

- [ ] **Step 5: Audit and commit the exact built assets**

Run:

```bash
docker ps -a --filter name='^/sshc-s3$' --filter name='^/sshc-sshd$' --format '{{.Names}} {{.Status}}'
test ! -e .integration-s3.json
git status -sb
git diff --check
git diff --stat
```

Expected: the only uncommitted paths are `internal/ui/dist/index.html` and old/new hashed assets; no test container or fixture credential file remains.

Commit them:

```bash
git add internal/ui/dist
git commit -m "build: refresh split web assets"
```

- [ ] **Step 6: Verify a clean linear main branch and push**

Run:

```bash
git fetch origin main
git status -sb
git diff --exit-code
git diff --cached --exit-code
git merge-base --is-ancestor origin/main HEAD
git push origin main
local_head=$(git rev-parse HEAD)
remote_head=$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')
test "$local_head" = "$remote_head"
```

Expected: clean `main`, fast-forward push, and identical local/remote commit hashes.
