# Section URL Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every primary sshc section a stable URL with native link, reload, bookmark, Back, Forward, and honest Not Found behaviour.

**Architecture:** Add a pure flat route mapping and a focused React History API hook, then make `App` derive its active section from that hook instead of local state. Primary navigation becomes real anchors while every programmatic section change uses the same `navigate` function. The existing Go SPA fallback remains unchanged.

**Tech Stack:** React 19, TypeScript 5.9, browser History API, Vitest and Testing Library, Playwright, Go HTTP SPA server

## Global Constraints

- URL state is limited to the eleven primary sections; connection selection, tabs, inspector contents, file targets, and forms remain in memory.
- Stable English slugs do not change with the selected UI language.
- `/` is Home; known non-root routes use no trailing slash.
- Unknown paths render a localized Not Found panel and are not rewritten.
- `/api` and `/api/*` remain outside the SPA fallback.
- Route operations never copy or persist the bootstrap fragment.
- No package is added.

---

## File Structure

- Create `web/src/routing/sectionRoute.ts`: pure Section identifiers, paths, and parser/formatter.
- Create `web/src/routing/sectionRoute.test.ts`: exhaustive route mapping and normalization tests.
- Create `web/src/routing/useSectionRoute.ts`: History API synchronization and navigation hook.
- Create `web/src/routing/useSectionRoute.test.tsx`: push, replace, and popstate behaviour tests.
- Modify `web/src/App.tsx`: consume the router, render links and Not Found, and centralize programmatic navigation.
- Modify `web/src/App.test.tsx`: assert URL-driven shell behaviour and update navigation semantics from button to link.
- Modify `web/src/i18n/messages.ts`: English and Japanese Not Found copy.
- Create `web/e2e/routing.spec.ts`: direct deep links, reload, browser history, trailing slash, and unknown URL coverage.
- Modify `web/e2e/support/environment.ts` and affected E2E specs: operate primary navigation as links.
- Modify `web/e2e/bootstrap.spec.ts`: expect reload to preserve the open section.
- Refresh `internal/ui/dist/**`: commit the production bundle generated from the routed UI.

---

### Task 1: Pure Section Route Contract

**Files:**
- Create: `web/src/routing/sectionRoute.ts`
- Create: `web/src/routing/sectionRoute.test.ts`
- Modify: `web/src/App.tsx` only to import the shared `Section` type after the contract passes

**Interfaces:**
- Produces: `sections`, `Section`, `sectionPath(section: Section): string`, `parseSectionPath(pathname: string): SectionRoute`.
- Produces: `SectionRoute = { kind: "section"; section: Section; canonicalPath: string; canonical: boolean } | { kind: "not-found"; pathname: string }`.
- Consumes: no application state or browser globals.

- [ ] **Step 1: Write the failing exhaustive mapping tests**

```ts
import { describe, expect, it } from "vitest";
import { parseSectionPath, sectionPath, sections } from "./sectionRoute";

const routes = [
  ["Home", "/"],
  ["Connections", "/connections"],
  ["Config", "/config"],
  ["Groups", "/groups"],
  ["Keys", "/keys"],
  ["Known Hosts", "/known-hosts"],
  ["Remote Keys", "/install-key"],
  ["Diagnostics", "/diagnostics"],
  ["Secrets", "/secrets"],
  ["Sync", "/sync"],
  ["History", "/history"],
] as const;

it.each(routes)("maps %s to %s in both directions", (section, path) => {
  expect(sectionPath(section)).toBe(path);
  expect(parseSectionPath(path)).toEqual({
    kind: "section", section, canonicalPath: path, canonical: true,
  });
});

it("keeps the route table exhaustive", () => {
  expect(routes.map(([section]) => section)).toEqual(sections);
});

it("accepts one trailing slash only as a non-canonical known path", () => {
  expect(parseSectionPath("/connections/")).toEqual({
    kind: "section", section: "Connections", canonicalPath: "/connections", canonical: false,
  });
  expect(parseSectionPath("/connections//")).toEqual({ kind: "not-found", pathname: "/connections//" });
});

it.each(["/missing", "/Connections", "/connections/child"])("rejects unknown path %s", (path) => {
  expect(parseSectionPath(path)).toEqual({ kind: "not-found", pathname: path });
});
```

- [ ] **Step 2: Run the mapping test and verify the missing module failure**

Run: `npm test --prefix web -- --run src/routing/sectionRoute.test.ts`

Expected: FAIL because `./sectionRoute` does not exist.

- [ ] **Step 3: Implement the pure route table**

```ts
export const sections = [
  "Home", "Connections", "Config", "Groups", "Keys", "Known Hosts",
  "Remote Keys", "Diagnostics", "Secrets", "Sync", "History",
] as const;

export type Section = (typeof sections)[number];

const paths: Record<Section, string> = {
  Home: "/",
  Connections: "/connections",
  Config: "/config",
  Groups: "/groups",
  Keys: "/keys",
  "Known Hosts": "/known-hosts",
  "Remote Keys": "/install-key",
  Diagnostics: "/diagnostics",
  Secrets: "/secrets",
  Sync: "/sync",
  History: "/history",
};

export type SectionRoute =
  | { kind: "section"; section: Section; canonicalPath: string; canonical: boolean }
  | { kind: "not-found"; pathname: string };

export function sectionPath(section: Section): string {
  return paths[section];
}

export function parseSectionPath(pathname: string): SectionRoute {
  for (const section of sections) {
    const canonicalPath = paths[section];
    if (pathname === canonicalPath) {
      return { kind: "section", section, canonicalPath, canonical: true };
    }
    if (canonicalPath !== "/" && pathname === `${canonicalPath}/`) {
      return { kind: "section", section, canonicalPath, canonical: false };
    }
  }
  return { kind: "not-found", pathname };
}
```

- [ ] **Step 4: Run the route tests**

Run: `npm test --prefix web -- --run src/routing/sectionRoute.test.ts`

Expected: all route tests PASS.

- [ ] **Step 5: Import `sections` and `Section` into App**

Delete the local `sections` tuple and `Section` alias from `App.tsx`, then add:

```ts
import { sections, type Section } from "./routing/sectionRoute";
```

Run: `npm run typecheck --prefix web`

Expected: PASS without changing runtime navigation yet.

- [ ] **Step 6: Commit the route contract**

```bash
git add web/src/routing/sectionRoute.ts web/src/routing/sectionRoute.test.ts web/src/App.tsx
git commit -m "feat: define stable section routes"
```

---

### Task 2: History API Router Hook

**Files:**
- Create: `web/src/routing/useSectionRoute.ts`
- Create: `web/src/routing/useSectionRoute.test.tsx`

**Interfaces:**
- Consumes: `parseSectionPath`, `sectionPath`, `Section`, and browser `window.location`, `window.history`, and `popstate`.
- Produces: `useSectionRoute(): { route: SectionRoute; navigate: (section: Section) => void }`.

- [ ] **Step 1: Write failing hook tests for initial parsing and navigation**

```tsx
import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useSectionRoute } from "./useSectionRoute";

afterEach(() => {
  window.history.replaceState(null, "", "/");
  vi.restoreAllMocks();
});

it("reads a direct deep link and pushes a new section URL", () => {
  window.history.replaceState(null, "", "/connections");
  const pushed = vi.spyOn(window.history, "pushState");
  const { result } = renderHook(() => useSectionRoute());
  expect(result.current.route).toMatchObject({ kind: "section", section: "Connections" });

  act(() => result.current.navigate("Keys"));
  expect(pushed).toHaveBeenCalledWith(null, "", "/keys");
  expect(window.location.pathname).toBe("/keys");
  expect(result.current.route).toMatchObject({ kind: "section", section: "Keys" });
});

it("reparses the real pathname on popstate", () => {
  const { result } = renderHook(() => useSectionRoute());
  act(() => {
    window.history.pushState({ untrusted: "state" }, "", "/history");
    window.dispatchEvent(new PopStateEvent("popstate", { state: { untrusted: "state" } }));
  });
  expect(result.current.route).toMatchObject({ kind: "section", section: "History" });
});
```

Add tests that `/connections/?source=test` becomes `/connections?source=test` with `replaceState`, an unknown path is not rewritten, and navigating to the already current path adds no entry while clearing a query with `replaceState`.

- [ ] **Step 2: Run the hook tests and verify the missing module failure**

Run: `npm test --prefix web -- --run src/routing/useSectionRoute.test.tsx`

Expected: FAIL because `./useSectionRoute` does not exist.

- [ ] **Step 3: Implement the hook with one synchronization path**

```tsx
import { useCallback, useEffect, useState } from "react";
import { parseSectionPath, sectionPath, type Section, type SectionRoute } from "./sectionRoute";

function readRoute(): SectionRoute {
  return parseSectionPath(window.location.pathname);
}

export function useSectionRoute(): { route: SectionRoute; navigate: (section: Section) => void } {
  const [route, setRoute] = useState<SectionRoute>(readRoute);

  useEffect(() => {
    const synchronize = () => {
      const next = readRoute();
      if (next.kind === "section" && !next.canonical) {
        window.history.replaceState(null, "", `${next.canonicalPath}${window.location.search}`);
        setRoute({ ...next, canonical: true });
        return;
      }
      setRoute(next);
    };
    synchronize();
    window.addEventListener("popstate", synchronize);
    return () => window.removeEventListener("popstate", synchronize);
  }, []);

  const navigate = useCallback((section: Section) => {
    const path = sectionPath(section);
    if (window.location.pathname === path) {
      if (window.location.search !== "" || window.location.hash !== "") {
        window.history.replaceState(null, "", path);
      }
    } else {
      window.history.pushState(null, "", path);
    }
    setRoute(parseSectionPath(path));
  }, []);

  return { route, navigate };
}
```

The implementation must not use `history.state` as route input and must not write a hash.

- [ ] **Step 4: Run route and hook tests**

Run: `npm test --prefix web -- --run src/routing/sectionRoute.test.ts src/routing/useSectionRoute.test.tsx`

Expected: all tests PASS.

- [ ] **Step 5: Commit History API synchronization**

```bash
git add web/src/routing/useSectionRoute.ts web/src/routing/useSectionRoute.test.tsx
git commit -m "feat: synchronize sections with browser history"
```

---

### Task 3: URL-driven Application Shell

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/App.test.tsx`
- Modify: `web/src/i18n/messages.ts`

**Interfaces:**
- Consumes: `useSectionRoute`, `sectionPath`, `Section`.
- Preserves: all existing `SectionView` and child `onNavigate(section)` contracts.
- Adds translations: `shell.pageNotFound`, `shell.pageNotFoundDescription`, `shell.goHome` in English and Japanese.

- [ ] **Step 1: Reset URL isolation and write failing App routing tests**

Change the `App.test.tsx` cleanup to restore `/` after every test:

```ts
afterEach(() => {
  window.history.replaceState(null, "", "/");
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});
```

Add tests that:

```tsx
it("renders a direct section URL and links every primary destination", async () => {
  window.history.replaceState(null, "", "/keys");
  render(<App bootstrap={resolvedBootstrap} health={resolvedHealth} vault={openVault} />);
  expect(await screen.findByText("keys panel")).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "Keys" })).toHaveAttribute("aria-current", "page");
  expect(screen.getByRole("link", { name: "Connections" })).toHaveAttribute("href", "/connections");
});

it("updates the URL for ordinary link and programmatic navigation", async () => {
  const user = userEvent.setup();
  render(<App bootstrap={resolvedBootstrap} health={resolvedHealth} vault={openVault} />);
  await user.click(await screen.findByRole("link", { name: "Connections" }));
  expect(window.location.pathname).toBe("/connections");
  await user.click(screen.getByRole("button", { name: "open pattern rule" }));
  expect(window.location.pathname).toBe("/config");
});
```

Add a popstate test, a locked deep-link test, a Not Found test that retains `/missing`, and a Japanese Not Found copy test.

- [ ] **Step 2: Run App tests and verify the routing assertions fail**

Run: `npm test --prefix web -- --run src/App.test.tsx`

Expected: FAIL because App still initializes Home from local state and renders primary navigation as buttons.

- [ ] **Step 3: Replace App local section state with the hook**

At the top of `App`:

```tsx
const { route, navigate } = useSectionRoute();
const section = route.kind === "section" ? route.section : null;
```

Delete `useState<Section>("Home")`. Replace every `setSection(target)` call with `navigate(target)`, including `openFile`, connection-creation detours, draft return, Overview callbacks, and `SectionView.onNavigate`.

Use `section` rather than the whole route as the inspector-clearing dependency so redundant navigation does not discard inspector content.

- [ ] **Step 4: Render primary navigation as semantic links**

For each primary item render:

```tsx
<a
  href={sectionPath(name)}
  aria-current={section === name ? "page" : undefined}
  onClick={(event) => {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    navigate(name);
  }}
>
  ...
</a>
```

Retain the existing visual classes and accessible group structure. Remove button-only `type` and `disabled` attributes; every route is enabled.

- [ ] **Step 5: Add and render localized Not Found state**

Add message keys:

```ts
"shell.pageNotFound": "Page not found",
"shell.pageNotFoundDescription": "No sshc section exists at this URL.",
"shell.goHome": "Go to Home",
```

and:

```ts
"shell.pageNotFound": "ページが見つかりません",
"shell.pageNotFoundDescription": "このURLに対応するsshcのセクションはありません。",
"shell.goHome": "ホームへ移動",
```

When `route.kind === "not-found"`, keep the shell and navigation visible, show the Not Found title in the shell header, and render a content panel with the unknown pathname plus a real `/` anchor whose ordinary click calls `navigate("Home")`.

- [ ] **Step 6: Update existing App tests from primary buttons to links**

Only navigation controls change role. Buttons inside panels, Inspector, and the connection-draft return action remain buttons. Replace queries such as:

```ts
screen.getByRole("button", { name: "Keys" })
```

with:

```ts
screen.getByRole("link", { name: "Keys" })
```

- [ ] **Step 7: Run App and routing tests**

Run: `npm test --prefix web -- --run src/routing/sectionRoute.test.ts src/routing/useSectionRoute.test.tsx src/App.test.tsx`

Expected: all tests PASS.

Run: `npm run typecheck --prefix web`

Expected: PASS.

- [ ] **Step 8: Commit the routed shell**

```bash
git add web/src/App.tsx web/src/App.test.tsx web/src/i18n/messages.ts
git commit -m "feat: route primary sections by URL"
```

---

### Task 4: Browser-level Routing Coverage

**Files:**
- Create: `web/e2e/routing.spec.ts`
- Modify: `web/e2e/support/environment.ts`
- Modify: `web/e2e/bootstrap.spec.ts`
- Modify: any E2E spec that directly queries a primary navigation item as a button

**Interfaces:**
- Consumes: the shipped binary, its existing SPA fallback, `openApplication`, `openSection`, and Playwright browser history.
- Produces: regression coverage for real navigation requests and session renewal on deep-link reload.

- [ ] **Step 1: Change `openSection` to operate the semantic link**

```ts
await page
  .getByRole("navigation", { name: "Primary" })
  .getByRole("link", { name, exact: true })
  .click();
```

Update direct primary-navigation queries in existing specs from role `button` to `link`. Do not change buttons that belong to panels.

- [ ] **Step 2: Write the browser routing scenarios**

Create a helper that preserves the one-use bootstrap fragment when replacing the first path:

```ts
function atPath(url: string, pathname: string): string {
  const source = new URL(url);
  const destination = new URL(pathname, source.origin);
  destination.hash = source.hash;
  return destination.toString();
}
```

Add Playwright tests that:

```ts
test("opens, reloads, and traverses section URLs", async ({ page, installation }) => {
  await openApplication(page, { url: atPath(installation.url, "/connections") });
  await expect(page).toHaveURL(/\/connections$/);
  await expect(page.getByRole("navigation", { name: "Connections" })).toBeVisible();

  await openSection(page, "Keys");
  await expect(page).toHaveURL(/\/keys$/);
  await openSection(page, "History");
  await page.goBack();
  await expect(page).toHaveURL(/\/keys$/);
  await page.goForward();
  await expect(page).toHaveURL(/\/history$/);
  await page.reload();
  await expect(page.getByRole("heading", { name: "History", level: 2 })).toBeVisible();
});
```

Add separate scenarios for `/connections/` normalization and `/missing` rendering Page Not Found without rewriting the URL, followed by the Home link.

- [ ] **Step 3: Update the old reload expectation**

In `bootstrap.spec.ts`, after opening Keys and reloading, assert the Keys panel remains visible without opening it a second time. Replace the comment that says section state is intentionally forgotten.

- [ ] **Step 4: Build and run routing E2E first**

Run: `make build && npm run e2e --prefix web -- e2e/routing.spec.ts e2e/bootstrap.spec.ts`

Expected: all selected Playwright tests PASS.

- [ ] **Step 5: Run the complete Playwright suite**

Run: `make e2e`

Expected: all platform-applicable tests PASS with only the existing OS-dependent skip.

- [ ] **Step 6: Commit browser coverage**

```bash
git add web/e2e
git commit -m "test: cover section URL navigation"
```

---

### Task 5: Production Bundle, Full Verification, Docker, and Push

**Files:**
- Modify: `internal/ui/dist/index.html`
- Replace generated hashed assets under: `internal/ui/dist/assets/`

**Interfaces:**
- Consumes: all prior tasks and the existing Makefile verification targets.
- Produces: a deterministic embedded UI, clean worktree, verified Docker integration, and pushed `main`.

- [ ] **Step 1: Verify generated API clients are unchanged**

Run: `make verify-generated`

Expected: PASS and no diff in `internal/api/models.gen.go` or `web/src/api/schema.d.ts`.

- [ ] **Step 2: Run all local tests**

Run: `make test`

Expected: Go standard and race tests, all Vitest tests, and TypeScript checks PASS.

- [ ] **Step 3: Build and commit the embedded production UI**

Run: `make build`

Expected: Vite and Go build PASS.

```bash
git add -A internal/ui/dist
git commit -m "build: refresh embedded routed UI"
```

- [ ] **Step 4: Run Docker integration using the CI services**

Run: `make integration-up`

Create the test bucket exactly as CI does:

```bash
docker exec sshc-s3 mkdir -p /data/buckets/sshc-test
curl -sS -X PUT http://127.0.0.1:8333/sshc-test -H 'Authorization: none'
```

Run: `make integration`

Expected: real SeaweedFS S3 and OpenSSH askpass integration tests PASS.

Always run: `make integration-down`

Expected: `sshc-s3`, `sshc-sshd`, and `.integration-s3.json` are removed.

- [ ] **Step 5: Rebuild and inspect the exact release diff**

Run: `make build && git diff --check && git status --short`

Expected: no uncommitted changes and no whitespace errors.

Run: `git diff --stat origin/main...HEAD`

Review that changes are limited to the approved spec, plan, route implementation, tests, messages, and embedded bundle.

- [ ] **Step 6: Push without overwriting remote work**

Run: `git fetch origin main && git rev-list --left-right --count origin/main...HEAD`

Expected: remote-behind count is `0`; local-ahead count is positive.

Run: `git push origin main`

Verify:

```bash
local_head=$(git rev-parse HEAD)
remote_head=$(git ls-remote origin refs/heads/main | awk '{print $1}')
test "$local_head" = "$remote_head"
git status --short --branch
```

Expected: local and remote hashes match and the worktree is clean.

- [ ] **Step 7: Confirm GitHub CI**

Use `gh run list --commit "$local_head"` to find the push-triggered CI run and wait for completion.

Expected: macOS, Go, End to end, Integration, Generated files, Web, and Dependency security all succeed.
