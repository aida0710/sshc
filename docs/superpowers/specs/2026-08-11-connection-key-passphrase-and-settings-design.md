# Connection-owned key passphrases and Settings

## Goal

An encrypted private key selected in a connection's Basic authentication form
can have its unlock passphrase stored or replaced there. The stored value is
owned by that key and cannot be reused by another key. Moving the login-item
toggle and master-password rotation out of Secrets into a new `/settings`
section makes Secrets describe only stored credentials and vault locking.

## Existing boundaries

- Account passwords and private-key passphrases remain different credential
  namespaces. A key passphrase can never be selected as a remote account
  password.
- A secret travels only from a password input to the local server. It never
  appears in a response, URL, browser storage, log, journal, preview, or diff.
- Changing the saved unlock value does not change the encryption passphrase in
  the private-key file. That existing operation remains on the Keys screen.
- The vault master password still unlocks every saved secret. Saving a key
  passphrase therefore trades the key's independent at-rest prompt for the
  convenience of one-action agent registration; the UI must not imply that the
  two protections remain independent.

## Chosen storage model

The vault gains `DedicatedKeyPassphrases`, parallel to the existing
`DedicatedPasswords` map:

```json
{
  "schemaVersion": 3,
  "dedicatedPasswords": { "host-alias": "..." },
  "dedicatedKeyPassphrases": { "keys/work/id_build": "..." }
}
```

The key is the workspace-relative private-key path. The value is structurally
separate from named `keyPassphrases`, so it cannot appear in a reusable picker
or be assigned to another key.

Vault schema version 3 accepts and migrates version 2 in memory by initialising
the new map empty. Its next successful write seals version 3. Version 1 remains
refused. Bumping the version is required: keeping version 2 would let an older
binary open a new vault, ignore the unknown map, and erase dedicated values on
its next write. Older binaries instead refuse version 3 safely.

The vault enforces mutual exclusion for a key subject:

- setting a dedicated value removes only that key's named assignment;
- assigning a named passphrase removes only that key's dedicated value;
- removing the key's saved passphrase removes either representation;
- moving a key relocates both named assignments and dedicated values in the
  same vault mutation;
- resolving a key passphrase checks the dedicated map before the named map.

Names and relative paths are not secret, but plaintext values remain sealed.
Vault status may report which relative paths have dedicated values; it never
returns the values.

## Connection update transaction

`UpdateConnectionRequest` gains a key-passphrase mutation with two states:

- `unchanged`
- `set_dedicated`, carrying the selected private-key ID and the new passphrase

An empty field means `unchanged`; removing a saved value is outside this
request's scope. The existing Keys screen remains the place to detach a stored
passphrase.

The server, not the browser, derives the private key's relative path from the
key ID. A `set_dedicated` mutation is accepted only when all of these hold:

1. the key exists in the current inventory and is a private key;
2. it is encrypted;
3. it is the single selectable `IdentityFile` produced by the connection after
   applying the same request's Basic-field changes;
4. the submitted passphrase actually decrypts that key;
5. the connection base snapshot and key bytes still match the snapshots that
   were validated when the transaction is committed.

Connection config changes, account-password changes, and the dedicated
key-passphrase change are applied to one cloned vault and one storage request.
The clone is published only after the complete disk commit succeeds. A
validation or write failure therefore leaves config bytes, sealed vault bytes,
and live memory unchanged. Replacing a shared named passphrase does not mutate
or delete that credential; only this key's reference is removed before its
dedicated value is stored.

## Connection UI

The Authentication card continues to begin with the SSH private-key selector.
The passphrase controls below it follow the selected key:

- no key, custom path, or multiple direct `IdentityFile` values: no editor;
- unencrypted key: explain that no passphrase is required;
- encrypted key with no saved value: show `New saved passphrase` and
  `Confirm saved passphrase`;
- encrypted key with a named assignment: show its name and whether other keys
  use it; explain that saving creates a value dedicated to this key and leaves
  the shared credential unchanged;
- encrypted key with a dedicated value: show `Saved for this key`; replacement
  still requires entering and confirming a complete new value.

No saved value is read back or prefilled. Changing the selected key, leaving
the connection, succeeding, or failing clears both password inputs. A mismatch
disables `Save Basic settings`. A wrong key passphrase is reported beside this
editor and nothing is changed. On success the editor returns to the stored
state and announces that only the selected key changed.

The passphrase mutation participates in the existing Basic save button. This
avoids a separate action that could save a passphrase for a newly selected key
even if the person later discarded the unsaved `IdentityFile` selection.

## Settings section

Add `Settings` as a routable primary section at `/settings`, in the Maintenance
navigation group with a settings icon. Direct loading, trailing-slash
canonicalisation, browser history, and not-found handling follow every other
section.

`SettingsPanel` owns two cards:

1. **Launch at login** — the existing supported-platform toggle and its failure
   handling, with the existing default-off behaviour unchanged.
2. **Master password** — current, new, and confirmation fields; the existing
   minimum length and snapshot-reseal result text remain unchanged.

Secrets keeps the credential metrics, named account passwords, named key
passphrases, add/delete controls, and `Lock vault`. It no longer renders or
owns state for launch-at-login or master-password rotation. Moving components
must not duplicate them on both pages.

## Error handling

- Wrong key passphrase: `403` problem specific to the selected key; clear the
  submitted values and preserve every stored/configured value.
- Key disappeared, became unencrypted, changed bytes, or stopped being the
  resulting connection key: conflict response; clear secret fields and reload
  before retrying.
- Vault or config commit failure: report that nothing changed and keep the
  connection selection stable.
- Settings load failure: show an actionable card-level error. Unsupported
  login items remain omitted, as today.
- Master-password rotation keeps the current distinction between a resealed
  live bucket snapshot and a local-only successful rotation.

## Testing

Backend tests cover:

- version 2 to version 3 migration and safe refusal by the version boundary;
- seal/open round trips without plaintext leakage;
- dedicated values cannot be listed or assigned as named credentials;
- named-to-dedicated and dedicated-to-named transitions affect one key only;
- relocation follows key moves;
- correct, incorrect, unencrypted, changed, missing, and unrelated key cases;
- a combined config/password/key-passphrase update is atomic on every injected
  commit failure and does not republish a failed clone;
- API responses and problems contain no submitted passphrase.

Web tests cover every Authentication-card state, secret-field clearing,
confirmation validation, the one-save request, server failures, `/settings`
routing, and the absence of the moved cards from Secrets.

End-to-end tests create an encrypted key, select it for a connection, save a
dedicated passphrase, replace it, and confirm one-action agent registration.
They also verify that a previously shared named passphrase remains assigned to
the other key. Settings routing, the login toggle where supported, and master
password rotation are exercised at their new location.

Full verification runs Go tests including the race detector, Web unit tests and
typecheck, Playwright end-to-end tests, generated-file verification, and the
Docker SeaweedFS/sshd integration suite.

## Non-goals

- Displaying or retrieving a stored secret.
- Changing the private-key file's encryption passphrase from Connections.
- Making a dedicated key passphrase reusable.
- Automatically saving a typed passphrase outside `Save Basic settings`.
- Changing launch-at-login defaults or master-password rotation semantics.
