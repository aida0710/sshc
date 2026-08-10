# Section URL Routing Design

## Outcome

Each primary section has a stable URL. Navigation, reload, bookmarks, and the
browser Back and Forward buttons open the expected section without exposing
selection or form state in the URL.

This is deliberately a flat section router. It does not route individual SSH
connections, configuration files, tabs, inspector contents, or in-progress
forms.

## Routes

| Path | Section |
| --- | --- |
| `/` | Home |
| `/connections` | Connections |
| `/config` | Config |
| `/groups` | Groups |
| `/keys` | Keys |
| `/known-hosts` | Known Hosts |
| `/install-key` | Install Key on Server |
| `/diagnostics` | Ad hoc checks |
| `/secrets` | Secrets |
| `/sync` | Sync |
| `/history` | History |

The slugs are stable application identifiers and do not change with the UI
language. `/` is the only Home URL. A known path with a trailing slash is
normalized with `history.replaceState`, so `/connections/` becomes
`/connections` without adding a history entry. Matching is otherwise exact and
case-sensitive.

Query strings are not route state. The initial page and trailing-slash
normalization retain an existing query string, but navigation within sshc writes
the canonical path without one. Route history operations never copy a hash. The
bootstrap fragment remains owned exclusively by `bootstrapSession`.

## Routing Boundary

A small routing module owns the bidirectional mapping between the existing
internal `Section` identifiers and paths. It exposes pure parsing and formatting
functions plus a React hook that:

- reads the initial section from `window.location.pathname`;
- uses `history.pushState` for application-initiated section changes;
- uses `history.replaceState` only for canonicalizing a known trailing-slash
  path;
- subscribes to `popstate` and updates the rendered section; and
- avoids adding another entry when the requested canonical path is already
  current.

No routing package is added. Eleven fixed, non-nested routes do not justify a
general-purpose router, and the Go HTTP server already returns the SPA document
for HTML navigation outside the API namespace.

The bootstrap exchange remains safe. `main.tsx` invokes `bootstrapSession`
before React is mounted, and that function synchronously consumes and removes a
valid `#bootstrap=...` fragment before starting its request. The section router
must never read, write, or reproduce that fragment.

## Navigation Behaviour

Primary navigation items become anchors with real `href` values. An ordinary
same-tab click is intercepted and routed through `pushState`; browser-native
modified clicks remain available for opening a section in another tab. The
selected anchor retains `aria-current="page"`.

All programmatic section changes use the same navigation function, including:

- Home actions that open Config or History;
- opening a configuration source location;
- leaving connection creation to create a Group or Key;
- returning to a saved connection draft; and
- any existing child-panel `onNavigate` callback.

The URL is the source of truth for the active section. In-memory state continues
to own selected connections, tabs, inspector state, file targets, and safe
connection-creation drafts. Changing sections continues to clear inspector
content as it does now.

If the vault is locked, the requested URL stays in the address bar. Unlocking
renders that requested section rather than forcing Home.

## Unknown Paths

An unknown path is not silently treated as Home and is not rewritten. Once the
shell is ready, its content area displays a localized Page Not Found panel with
a link to Home. The primary navigation remains available, so choosing any
section creates a normal history entry from the unknown URL. Back can therefore
return to the Not Found state honestly.

The Go server continues to reserve `/api` and `/api/*`; those paths must never
fall back to the SPA document.

## Failure Handling

History API operations are synchronous browser operations and need no retry UI.
Pure parsing rejects unrecognized, differently cased, or multiply nested paths
as unknown. A `popstate` event always reparses the actual pathname instead of
trusting arbitrary `history.state` supplied by another script.

Routing does not persist or serialize credentials, passwords, private-key
material, form fields, or SSH host selections.

## Testing

Unit tests cover:

- every Section-to-path and path-to-Section mapping;
- Home, exact matching, trailing-slash normalization, and unknown paths;
- initial deep links and locked-then-unlocked deep links;
- navigation using `pushState` without a page reload;
- Back and Forward handling through `popstate`;
- programmatic navigation using the same URL path;
- no duplicate history entry for the current section; and
- a localized Not Found panel that links to Home.

End-to-end tests open `/connections` directly, move between multiple sections,
exercise Back and Forward, reload a deep link, unlock on a deep link, and verify
an unknown path. Existing server tests continue to prove that HTML deep links
receive `index.html` while missing API routes do not.

The normal generated-file check, Go tests including race detection, web unit
tests and typecheck, production build, Playwright suite, and Docker integration
remain the final verification boundary.

## Non-goals

- Connection aliases such as `/connections/bastion`.
- Config file names, line numbers, search terms, or selected editor tabs.
- Inspector open state or contents.
- Modal and form state.
- Redirects from speculative legacy slugs.
- Adding React Router or another package.
