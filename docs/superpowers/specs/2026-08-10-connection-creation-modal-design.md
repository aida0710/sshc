# Connection creation modal

## Outcome

Creating a connection must leave it ready to open and must immediately show
that connection in sshc. The current inline form is not sufficient: it accepts
only an alias and an already existing file, writes `HostName` equal to the
alias, omits the user and authentication, and cannot target a declared group
whose directory contains no `.conf` file.

The Connections screen will replace that inline form with one modal. A single
server operation will validate and commit the SSH configuration and, when
password authentication is chosen, the encrypted password-vault change. A
successful response closes the modal, reloads the tree, selects the new host
and opens its Basic detail tab. It does not launch a terminal.

## Modal

The modal contains two sections.

### Connection

- **Connection name** is the OpenSSH alias and is required. It uses the
  existing safe alias rules: 1 through 64 ASCII letters, digits, dots, dashes
  or underscores, beginning with a letter or digit.
- **Save in** lists "No group" followed by every declared group from
  `Overview.groups`, including empty and nested groups such as
  `home-lab/others`. It does not derive destinations from `Overview.files`.
- **HostName / IP address** is required and accepts a safe DNS name, IPv4
  literal or unbracketed IPv6 literal.
- **User** is optional. An empty value omits `User`, allowing OpenSSH to use the
  local login name.
- **Port** is optional in the form. Submission normalises an empty value to
  `22`, and every created block explicitly contains `Port 22`.

For a declared group, the canonical destination is
`connections/<group>/<alias>.conf`; the directory is created inside the same
transaction when it is absent. With no group, the block is appended to the
entry configuration file reported by the server. An existing destination file
or an alias already declared anywhere in the reachable Include graph is a
conflict and is never overwritten.

### Authentication

The user chooses one of two methods.

**Password** offers three mutually exclusive sources:

1. **For this connection** accepts a password and stores it in the encrypted
   vault as a dedicated password owned by the connection alias. It is
   persistent, is not returned by the reusable credential listing and cannot
   be assigned to another connection.
2. **Saved password** assigns an existing credential from the `password`
   namespace. Key passphrases must never appear in this list.
3. **New shared password** accepts a new credential name and value, stores it
   in the `password` namespace and assigns it to the new alias.

The vault must exist and be unlocked before submission. When it does not exist,
the modal offers initialisation with a master password; when it is locked, the
modal offers unlock. Connection creation remains disabled until that succeeds.
These preparatory vault operations do not create a connection.

**SSH key** lists only private-key entries from the existing key inventory.
Selecting one writes its workspace-relative path as `IdentityFile`. Public
keys, certificates, unknown files, trash entries and invalid keys are not
eligible. Key generation remains on the Keys screen and is outside this
feature.

## API and transaction

Add `POST /api/v1/connections` with a discriminated request. Common fields are
`alias`, `group`, `hostName`, optional `user`, and optional `port`. The
authentication object is one of:

- `{ "kind": "dedicated_password", "password": "..." }`
- `{ "kind": "saved_password", "credential": "..." }`
- `{ "kind": "new_shared_password", "credential": "...", "password": "..." }`
- `{ "kind": "identity_file", "keyId": "..." }`

The response contains the committed transaction ID, save preview and created
host identity. It never contains a password.

The application layer owns the use case. Before staging anything it validates
the alias, host name, user, port, declared group, destination collision, alias
collision, selected private key, credential kind and vault state. It renders a
new block using configuration-model values rather than string concatenation:

```sshconfig
Host <alias>
    HostName <hostName>
    User <user>          # omitted only when the form was empty
    Port <port>          # always present; empty form input becomes 22
    IdentityFile <path>  # key authentication only
```

For password authentication, the configuration contains no secret. The vault
stores a dedicated password or stores/assigns a reusable password to the
alias. Dedicated passwords have their own sealed-vault collection rather than
being encoded as specially named reusable credentials; this keeps the
non-reusable rule structural and avoids a name-prefix convention. The
configuration change,
directory creation and newly sealed vault bytes are committed through one
storage transaction. Validation, locking and commit order must prevent a
concurrent config save or vault save from losing either side. A failed request
changes neither the configuration nor the vault.

## Client flow

The existing New connection button opens an accessible modal. Cancel, Escape
or successful completion unmounts it and clears every field. The primary
button is enabled only when the visible branch is complete and the vault is
ready when required.

On a rejected create, the modal stays open and retains all non-secret fields.
Any typed account password or master password is cleared. The error appears by
the relevant field when the server supplies a field, otherwise as a modal-level
notice. A configuration conflict asks the user to reload and resubmit; it does
not silently retry against new bytes.

On success the page reloads the overview, closes the modal, selects the
returned identity, loads its detail and leaves the creation diff visible in the
Save preview. The selected host opens on the Basic tab. There is no automatic
terminal launch.

## Validation and errors

The server is authoritative even when the client has already validated:

- alias: existing application and platform safe-alias constraints;
- host name: existing safe host-name constraint;
- user: no empty-after-trim value when supplied, control characters, newline
  or OpenSSH whitespace;
- port: blank becomes 22; otherwise an integer from 1 through 65535;
- group: exact member of the declared group set, or empty for no group;
- key: existing private-key inventory item inside the workspace;
- credential: existing `password` credential, never `key_passphrase`;
- password and credential names: existing vault size and naming limits;
- alias and path: no reachable duplicate alias and no destination overwrite.

Stable problem codes distinguish invalid fields, alias conflict, path
conflict, undeclared group, vault missing, vault locked, unknown credential,
invalid key and external configuration conflict.

## Verification

Backend tests cover request decoding, validation, exact rendered directives,
blank-port normalisation, group destination creation, no-group append, each
authentication branch, duplicate rejection and atomic failure. Tests compare
both configuration and sealed-vault bytes before and after every rejected
operation.

Frontend tests cover the modal, all declared group options including an empty
nested group, conditional authentication controls, vault initialisation and
unlock, secret clearing, request shape, error placement and automatic detail
selection.

End-to-end tests use an isolated SSH home to create one key-authenticated and
one dedicated-password connection. They verify the filesystem result, the
selected detail view and the absence of password material from responses,
metadata, logs and the DOM after the modal closes. Existing macOS and Linux
test suites must continue to pass. No package is added.
