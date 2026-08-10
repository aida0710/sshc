# Workflow Discoverability Design

## Outcome

Make the next useful action obvious in the connection, key and group workflows
without flattening SSH concepts or weakening sshc's transaction and secret
boundaries. A selected connection and editor tab are addressable by URL, common
connection organisation controls stay with the Basic editor, newly generated
keys lead into assignment or remote installation, group movement is named
consistently, and staged group changes keep their save action in view.

No dependency is added. Passwords, passphrases, private-key material and
in-progress secret fields never enter URLs, shell workflow state or browser
history.

## Connection URL state

The section path remains `/connections`. A concrete connection uses a canonical
query string:

```text
/connections?path=connections%2Fwork%2Fapi.conf&host=api&tab=basic
```

`path` is the editable workspace-relative configuration path, `host` is the
exact alias, and `tab` is one of `basic`, `jump`, `advanced`, `raw`, `effective`
or `diagnostics`. All three values are required for a concrete target; an
unknown tab falls back to `basic`, while a partial identity is treated as the
unselected `/connections` page.

Path plus alias is used because alias alone is not unique. Adding a persisted
stable identifier would require a metadata migration and would still need
reconciliation after hand editing, so it is outside this change. The relative
path is non-secret but may reveal local naming; no absolute path is placed in
the URL.

Selecting a host and changing tabs push history entries. Back and Forward
restore both the selected host and tab. Creating, renaming or moving a host
replaces the current connection URL with the new identity so the successful
operation does not leave a stale history entry. Deleting a host replaces it
with `/connections`. An unavailable deep link remains visible, reports that the
target no longer exists and offers a return to the connection list.

The existing section router owns browser-location synchronization. It exposes a
location-aware internal navigation function rather than letting individual
components call `history` independently. Primary navigation to Connections
clears connection query state and therefore returns to the connection list.

## Connection information architecture

The six editor tabs retain their SSH-specific responsibilities. The
Organisation controls are rendered only inside Basic, directly after the
Connection and Authentication cards, instead of appearing below every tab.
Group, alias and comment operations keep their current server transactions and
individual buttons; their section explicitly says that each action saves
independently. This avoids pretending a rename, file move and encrypted vault
update are one reversible form submission when the APIs do not have that
contract.

The shell inspector becomes contextual. For a selected connection it is named
**Display and classification** and contains favourite, colour, tags and display
order. It states that these app-only fields save immediately. For a selected
group it is named **Group display settings** and states that its fields are
staged until **Save groups**. The generic **Details** label is not used for
editable metadata.

Normal organisation uses **Primary group**. Connection rows show a visible drag
handle while grouped, and the list explains that rows can be dragged between
groups. The separate disclosure remains **Advanced file actions**, but its move
control is named **Change storage file** and explains that this changes the
underlying SSH config file, not the primary group.

## Key workflow

The Keys header contains a real `#create-key-heading` action so the creation form
is reachable without crossing the inventory, agent and warning sections.

After in-process key generation succeeds, a next-steps card identifies the new
key and offers:

- **Assign to a connection**: navigate to Connections with only the generated
  private-key ID and relative path in transient shell state. The page asks the
  user to choose a connection. Once selected, Basic preselects the generated key
  as an unsaved draft; the user must still press **Save Basic settings**.
- **Install on a server**: navigate to Install Key on Server with only the
  generated public-key relative path in transient shell state. The destination
  panel resolves that path against its fresh public-key inventory and preloads
  the public key. Planning and registration remain explicit actions.

The shell state contains IDs and relative paths only. It is cleared when the
suggestion is applied, dismissed, or superseded. Hardware-key command generation
does not offer these actions because no key exists in sshc yet.

## Group workflow

The Add group card moves above the group list. New groups, colour, display order
and shared-setting changes remain staged. Rename and remove remain immediate
filesystem transactions and retain their existing local labels and confirmation.

Whenever staged changes exist, a sticky action bar remains at the bottom of the
scrolling page with **Preview group changes** and **Save groups**. It explicitly
contrasts staged changes with immediate rename/remove. A clean page shows only
the normal saved-state hint and no sticky action bar. Navigating away still does
not silently save.

## Errors and safety

- Parsing malformed or partial connection URL state never initiates a request
  with an empty identity.
- Deep-link load failure does not create, rename, move or delete anything.
- Suggested keys are resolved against fresh inventories and cannot introduce a
  caller-supplied filesystem path into `IdentityFile`.
- Preselection is a draft only. No key is assigned and no remote host is
  contacted until the existing explicit save/plan/register actions occur.
- Secret inputs retain their existing reset-on-navigation and reset-on-result
  behavior.

## Verification

Unit tests cover connection query parsing/formatting, location synchronization,
selection and tab history, stale deep links, contextual inspector labels,
Basic-only organisation controls, group/file movement copy, generated-key next
steps, safe key preselection, remote public-key preloading, top creation links,
and the group sticky save bar.

End-to-end tests cover direct connection/tab deep links, Back/Forward between
connections and tabs, URL replacement after identity changes, and the generated
key assignment/installation handoffs. Final verification includes generated
assets, TypeScript, web unit tests, Go normal and race tests, Playwright, and the
Docker-backed SeaweedFS/sshd integration suite.

## Non-goals

- No stable-ID metadata migration or nested `/connections/:alias` route.
- No URL persistence for drafts, passwords, passphrases, search terms, inspector
  open state or key workflow suggestions.
- No backend transaction that combines Basic fields, rename, group move,
  comment and app metadata into one save.
- No mandatory wizard that replaces the independent Keys, Connections or
  Install Key on Server screens.
