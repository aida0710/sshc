# Navigation and Workflow Usability Design

## Goal

Remove the dead ends and ambiguous actions found in the post-implementation
design review without weakening sshc's secret-handling or explicit-action
guarantees.

## Selected approach

Use a small shell-owned workflow state rather than a router rewrite or a set of
text-only patches. The shell retains only the non-secret portion of an unfinished
connection draft while the user visits Keys or Groups. Returning to Connections
reopens the modal with that draft. Passwords, vault passphrases and confirmations
are never lifted into shell state and must be entered again.

Text-only links were rejected because they still discard the form. A new URL
router and multi-step wizard were rejected because the application is currently a
single-window section state machine and neither is needed to solve these flows.

## Connection creation

- `CreateConnectionModal` accepts an optional non-secret draft and can hand that
  draft to a prerequisite navigation callback.
- The group field offers a contextual Manage groups action. The private-key
  branch offers Create a key when no eligible private key exists.
- App stores the handed-off draft and shows a persistent return bar on Keys and
  Groups. Returning to Connections reopens the modal.
- Cancel and successful creation discard the draft. Navigating for a prerequisite
  clears every secret by unmounting the modal and stores no secret outside it.
- When an eligible private key exists and the user has not selected another
  method, SSH private key is selected and listed first. Without a private key,
  the dedicated encrypted password remains the usable default.
- Connection name and Host name visibly say Required. A disabled primary action
  has adjacent text explaining what remains incomplete. Port copy says that the
  default is 22 rather than describing the storage representation.

## Navigation and diagnostics

- The connection-specific Diagnostics tab remains the normal route for a known
  connection.
- The independent sidebar section is renamed Ad hoc checks and keeps accepting an
  arbitrary SSH target. It also offers known aliases through a datalist so users
  do not have to remember or retype them.
- Home's workspace-attention action routes configuration problems to Config and
  pending transactions to History. It no longer sends an unscoped problem to an
  empty diagnostics form.
- The inspector toggle includes visible Details text on normal desktop widths,
  while preserving its existing accessible name and attention indicator.

## Action state and connection management

- Save fields, Save raw block, Save comment, Rename, Move to group, group Preview,
  group Save and file Move are disabled until they have a meaningful change.
- File-level duplicate, move and delete remain behind the disclosure, renamed
  Advanced file actions. Normal organisation continues to use Primary group.
- Group rename and remove remain immediate because they are filesystem
  transactions with existing confirmation and history semantics. They receive a
  visible Writes immediately label beside those controls; staged operations remain
  under the Save groups action.

## Notice and inventory clarity

- Pattern- and host-detail notices are removed from the page-level Connections
  notice strip. Pattern rules remain explained in the tree and host-specific
  notices remain in Host detail and Effective.
- The Keys metric is renamed Classified SSH files because its value includes
  config files and other classified workspace entries, not only keys.
- Remote Keys is renamed Install Key on Server to state its action rather than
  expose the implementation's direction.

## Verification

- Component tests cover draft handoff/resume, no secret retention, prerequisite
  links, key-first default, disabled no-op actions, diagnostic alias suggestions,
  scoped notices and revised labels.
- App tests cover the shell return bar and Home destinations.
- End-to-end tests cover creating a key through the modal detour and resuming the
  connection without losing non-secret fields.
- Run generated verification, build, full unit/race/type tests and Playwright.

## Non-goals

- No URL router, key generation inside the connection modal, backend transaction
  changes or new dependency.
- No secret may be stored in App, localStorage, sessionStorage, URL state or
  browser history.
