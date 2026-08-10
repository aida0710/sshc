# Connection Basic editing

## Outcome

The Basic tab of an existing connection must work as a stable connection
form, not merely as a list of directives that happen to exist in that Host
block. A user must always be able to see and edit the host name or IP address,
user, port, SSH private key and stored-password association from the selected
connection's detail screen.

The form keeps SSH configuration and encrypted secrets as separate data
types. It presents them together because they belong to one user task, but it
never places a password in `ssh_config`, a preview, a response, a log or a
prefilled DOM value. Advanced, Raw, Effective and Diagnostics remain available
for configuration that cannot safely be represented by the common form.

Connection alias changes, group moves and comments remain in the existing
Organisation section. Combining file moves and renames with this feature would
make the common save unnecessarily broad and is outside this change.

## Basic form

The Basic tab has two cards: **Connection** and **Authentication**. It no
longer renders every `basic`-category directive as an undifferentiated row.
Directives outside the stable form, including `IdentitiesOnly`,
`AddKeysToAgent` and `Tag`, remain editable in Advanced. Jump, Raw, Effective
and Diagnostics retain their current responsibilities.

### Connection

The Connection card always renders these controls in this order:

- **Host name or IP address**
- **User**
- **Port**

Each control distinguishes one of three origins:

1. **This connection** means the selected Host block contains exactly one
   directive for the keyword. Editing updates that directive.
2. **Inherited** means the approximate effective projection found the winning
   value in another matching block. Editing creates a directive in this Host
   block. The UI names the source file and line.
3. **SSH default** means no configured value exists. Host name displays the
   alias, port displays `22`, and user displays an empty value with a hint that
   OpenSSH will use the local login name. Editing creates a directive.

An unchanged inherited or default value is not written merely because the
form was saved. A **Use inherited/default value** action removes the one direct
directive. It never removes a directive from a different Host block.

Host name must be a safe DNS name, IPv4 literal or unbracketed IPv6 literal
when set. User is optional and cannot contain whitespace or control
characters. Port is an integer from 1 through 65535. Clearing User requests
inheritance; Host name and Port use an explicit inherit/default action rather
than treating an accidental empty field as deletion.

If the selected block contains duplicate HostName, User or Port directives,
the corresponding control remains visible but is read-only and explains that
multiple direct values must be resolved in Advanced. Basic never guesses
which duplicate should be rewritten or silently deletes the others.

### Authentication

OpenSSH can try keys and passwords in the same connection, and inherited
`IdentityFile` directives are cumulative. The UI therefore must not represent
authentication as a factually incorrect exclusive key/password switch. The
Authentication card contains independent **SSH private key** and **Stored
password** sections.

The SSH private-key control shows:

- the one directly configured `IdentityFile`, when it maps to an inventoried
  private key;
- **SSH agent or inherited keys** when there is no direct IdentityFile;
- a read-only custom-path state when the direct path is not an inventoried
  private key; or
- a read-only complex state when the block contains multiple direct
  IdentityFile directives.

Choosing an inventoried private key adds or replaces the single direct
IdentityFile. Choosing **SSH agent or inherited keys** removes the single
direct IdentityFile. It does not remove inherited IdentityFile directives.
Public keys, certificates, trash entries and unknown paths cannot be selected.
Custom and multiple direct paths remain editable in Advanced.

The stored-password section reports four states without revealing a secret:

- vault absent: offer the existing vault-initialisation flow;
- vault locked: offer unlock and do not claim whether this connection has a
  password;
- no password assigned: offer a dedicated password, an existing reusable
  password, or a new reusable password;
- password assigned: show only that an assignment exists and, for a reusable
  credential, its name.

An empty password field means no change. Replacing a password requires an
explicit **Replace password** action. Removing it requires a separate
**Remove stored password** action and confirmation. Removing a dedicated
password deletes that connection-only secret. Removing a reusable password
only unassigns this connection; it does not delete a credential still usable
by other connections.

The existing password-eligibility blockers and warnings remain authoritative.
For example, `PasswordAuthentication no` blocks adding or replacing a stored
password, while an IdentityFile is a visible warning rather than a blocker.
The Diagnostics tab continues to run connection checks, but it no longer
duplicates the password-management panel.

## Form state and save behaviour

The Basic tab has one **Save Basic settings** button. It is enabled only when
a connection field, direct key selection or password assignment has changed.
The client submits only explicit changes; merely opening the page cannot
materialise inherited values into the Host block.

Configuration-only changes remain saveable while the vault is absent or
locked. A password change requires an existing unlocked vault. Vault
initialisation and unlock are preparatory operations and do not save Basic
settings by themselves.

On success the page reloads the overview and selected Host detail, keeps the
same connection selected, resets dirty state and clears every typed secret.
The committed configuration diff remains visible in the existing Save preview.
Vault bytes and encrypted-vault diffs are never included in that preview.

On failure the form stays open. Non-secret drafts remain so the user can
correct or retry them; password and vault-passphrase inputs are cleared.
Validation errors appear beside the corresponding control. Configuration or
vault conflicts explain that nothing was changed and require a reload rather
than silently rebasing onto bytes the user did not review.

Changing connection selection, browser section or detail identity unmounts or
resets the form and clears all secret inputs. Password text is never restored
from component state, browser history or an API response.

## API contract

Add `PATCH /api/v1/connections`. The request identifies the existing Host
block and carries its exact base file bytes for optimistic concurrency:

```json
{
  "identity": { "path": "connections/home/nas.conf", "alias": "nas" },
  "base": "Host nas\n\tHostName 192.0.2.10\n",
  "hostName": { "action": "set", "value": "192.0.2.11" },
  "user": { "action": "inherit" },
  "port": { "action": "set", "value": 22 },
  "identityFile": { "action": "set", "keyId": "0123456789abcdef0123456789abcdef" },
  "password": { "kind": "unchanged" }
}
```

`hostName`, `user`, `port` and `identityFile` are optional; omission means
unchanged. Each connection-setting change is a discriminated `set` or
`inherit` object. HostName/User set values are strings, Port set value is an
integer and IdentityFile set carries an inventoried private-key ID.

`password` is required so accidental omission cannot acquire a future default.
It is one of:

- `{ "kind": "unchanged" }`
- `{ "kind": "dedicated_password", "password": "..." }`
- `{ "kind": "saved_password", "credential": "..." }`
- `{ "kind": "new_shared_password", "credential": "...", "password": "..." }`
- `{ "kind": "remove" }`

The response uses the existing `SaveResult`: transaction ID, written paths and
configuration-only Save preview. It contains neither the password nor sealed
vault bytes.

The client must reject an update with no actual change. The server repeats
that check and remains authoritative for identity, base, field, key, password,
credential and vault validation. Unknown JSON fields, invalid union branches,
oversized strings and conflicting fields are rejected.

## Application and transaction boundary

The application layer owns `UpdateConnection`. It resolves the current graph,
locates the exact Host block, compares the supplied base with disk and derives
line edits on the server. The browser does not send line numbers for common
fields. For each changed keyword, the server permits zero or one direct
directive and rejects a duplicate direct form as a complex connection instead
of flattening it.

Setting a missing directive appends one through the existing parsed-config
editing primitives. Setting a single direct directive rewrites only that line.
Inheriting removes only that direct directive. Existing indentation, comments,
line endings, unknown directives and unrelated Host blocks remain byte-for-byte
stable.

An IdentityFile key ID is resolved against the current server-side private-key
inventory. The browser never supplies a filesystem path. Inheritance removes
the one direct IdentityFile only; multiple direct values are rejected.

When password is `unchanged`, the operation does not require or write the
vault. When it changes, extend the existing password-mutation transaction
boundary with `remove`: clone the unlocked vault, prepare the config change,
and commit the sealed vault change and any SSH-config change through one
journalled `storage.Request`. Publish the cloned in-memory vault only after the
storage commit succeeds. A config conflict, vault conflict, invalid key,
unknown credential or storage failure changes neither side.

A password-only update still verifies that the target Host and supplied base
exist and have not changed, then commits only the sealed vault file. Its public
preview contains no file diff. A configuration-only update uses the existing
configuration transaction path. A request that produces neither a config nor
password change is rejected.

## Client components

Introduce a focused `ConnectionBasicForm` component rather than adding more
state to `HostDetailPanel`. It receives Host detail, current key inventory and
integration APIs, derives the three value origins, owns non-secret and secret
drafts, and emits one typed update request. Pure helpers derive a basic field's
display value, source, direct-line cardinality and update operation; these are
unit tested independently.

`HostDetailPanel` continues to own tab selection and the non-Basic editors. It
renders `ConnectionBasicForm` for Basic and removes `PasswordPanel` from
Diagnostics. `ConnectionsPage` owns the network mutation, reload and Save
preview exactly as it does for existing configuration saves.

The key picker and password choices reuse the existing key inventory, vault
status and credential APIs used by the creation modal. No new package is
introduced. Shared logic may be extracted only when both existing and new
callers need exactly the same behaviour; this feature does not require a broad
modal refactor.

## Errors and boundaries

Stable problem codes distinguish:

- invalid request or no change;
- Host block missing or externally changed;
- duplicate/complex direct field;
- selected private key missing or invalid;
- vault missing or locked;
- password ineligible;
- reusable credential missing;
- vault/config transaction conflict; and
- storage or reload failure.

Read-only external files and non-simple wildcard-only Host blocks remain
outside Basic editing. Duplicate alias notices remain visible. The feature
does not run `ssh -G`, connect to a host, change `known_hosts`, rename a
connection, move a group, alter a reusable credential's value globally or
delete a reusable credential.

## Verification

Application tests cover add/set/inherit for HostName, User, Port and a single
IdentityFile; default preservation; duplicate rejection; current key
inventory validation; password unchanged/add/replace/assign/new-shared/remove;
dedicated deletion versus reusable unassignment; password-only updates; locked
vault behaviour; stale base conflicts; and atomic failure with unchanged
config, sealed-vault bytes and in-memory vault.

HTTP and contract tests cover every discriminated request branch, unknown
field rejection, bounds, stable problem mappings and responses without secret
or sealed-vault material.

Frontend tests cover fields that are direct, inherited and default; stable
rendering when a directive is absent; duplicate and custom-key read-only
states; key selection; every vault state; empty password meaning unchanged;
explicit replace/remove; secret clearing; one-button request construction;
field-level errors; reload after success; and password removal from
Diagnostics.

End-to-end tests edit an existing sparse Host block, add User and explicit Port,
change an inventoried key, store and then remove a dedicated password, reload
the page and verify the resulting filesystem and visible form. They also force
a transaction failure and verify that neither configuration nor vault changes.
Existing Go, race, Vitest, TypeScript, build, generated-contract and Playwright
suites must continue to pass. Dependency manifests must remain unchanged.
