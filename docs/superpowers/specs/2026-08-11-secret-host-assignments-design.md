# Secret host assignments design

## Goal

The Secrets page must make each stored account password and key passphrase's
relationship to SSH hosts explicit. Today an account password's raw `uses`
array is rendered without a label, while a key passphrase reports only the key
paths that point to it. The new view shows labelled host assignments without
ever returning a secret value.

## User experience

Each reusable account password shows:

- its credential name;
- `Assigned hosts`, followed by the concrete SSH aliases that use it; or
- `No assigned hosts` when it is unused.

Each reusable key passphrase shows:

- its credential name;
- `Keys`, followed by every workspace-relative private-key path assigned to it;
- `Assigned hosts`, containing the union of concrete SSH aliases that resolve
  to those keys; or
- the corresponding empty-state text for keys or hosts.

A key-dedicated passphrase also appears in the key-passphrase section. It is
labelled as dedicated, shows its one key path and the hosts that resolve to that
key, and retains the existing dedicated-passphrase removal operation. It is not
presented as a reusable named credential.

The host list is described as the set that sshc can confirm from the loaded SSH
configuration. sshc expands inherited directives and wildcard rules such as
`Host *` across the concrete aliases declared in the loaded graph. It does not
invent aliases for an open-ended pattern such as `Host build-*`, evaluate
`Match exec`, or guess the meaning of relative and tokenised `IdentityFile`
values. The existing key-reference diagnostics remain the place to investigate
those unresolved forms.

## API contract

`GET /api/v1/credentials` remains the one read used by the Secrets page. Its
named `Credential` objects gain a required `hosts` array:

- for `password`, `uses` and `hosts` contain the assigned host aliases;
- for `key_passphrase`, `uses` contains assigned key paths and `hosts` contains
  the deduplicated aliases that resolve to any of those keys.

`CredentialList` gains a required `dedicatedKeyPassphrases` array. Each item
contains only a workspace-relative `key` path and its `hosts` array. It contains
no credential name and cannot be selected for another key.

All arrays are non-null, sorted, and deduplicated. No password, passphrase,
master password, ciphertext, prompt content, or value-derived metadata enters
the response.

## Server-side data flow

The password handler first reads the unlocked vault's named credentials and
dedicated key paths. It collects the key paths used by named and dedicated
passphrases, then asks the configuration service for one key-to-host index.

The configuration service resolves the current Include graph, enumerates its
concrete non-duplicate host aliases, and computes each alias's effective
configuration. For every effective `IdentityFile`, it uses the keys package's
existing conservative path rules to compare that value with the requested key
paths. A match adds the alias to that key's host set. This calculation is
read-only and is performed on every credentials read, so connection edits,
group changes, and key relocation cannot leave a persisted usage index stale.

The HTTP layer joins the vault relationship with the key-to-host index and
returns the API model. The vault remains unaware of SSH configuration, and the
frontend remains unaware of configuration parsing.

If the Include graph cannot be resolved, the credentials request fails with a
`credential_usage_unavailable` problem rather than returning an authoritative-
looking empty host list. The page translates that code into an actionable
reload/configuration notice while retaining the last successfully loaded list.

## UI structure

The current compact inline list becomes a stacked item per credential. Labels
and lists are semantic text, not inferred by position:

```text
DubGuild
Assigned hosts
- tv-recoding

miyabi-g (dedicated)
Keys
- keys/miyabi-g
Assigned hosts
- tv-recoding
- encode-worker
```

Existing store and delete controls remain in their current sections. Shared
credential deletion still uses the named credential endpoint. Dedicated
passphrase removal uses the existing unassign endpoint with the key path as its
subject.

English and Japanese message catalogs receive explicit labels and empty-state
copy. Secret values are never prefilled or rendered.

## Testing

- Application unit tests cover direct `IdentityFile`, inherited `Host *`, one
  key used by multiple hosts, multiple keys sharing a passphrase, stable
  deduplication, and an unresolved relative path that must not produce a guess.
- HTTP tests cover password hosts, named passphrase keys and hosts, dedicated
  passphrase entries, deterministic empty arrays, usage-lookup failure, and
  absence of secret values.
- API-client tests reject missing or malformed host-assignment fields.
- React tests verify labelled host lists, key lists, both empty states, and the
  dedicated removal operation.
- Existing full Go, race, Web, generated-contract, E2E, and Docker integration
  suites remain green.

## Scope boundaries

This change does not expose secret values, change vault assignment semantics,
edit SSH configuration, create new aliases from wildcard patterns, evaluate
executable SSH directives, or add filtering/search to the Secrets page.
