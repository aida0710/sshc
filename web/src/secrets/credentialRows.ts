import type { Credential, CredentialKind, CredentialList } from "../api/credentials";
import type { Translate } from "../i18n/context";

type DedicatedKeyPassphrase = CredentialList["dedicatedKeyPassphrases"][number];

// CredentialRow is one line of a Vault page: a saved credential of the page's kind, or, on the key
// passphrases page, a passphrase saved for one key only. That one has no name of its own: the row shows the
// key's file name, and label (what the row is called for screen readers) is the key's whole path.
export type CredentialRow =
  | { id: string; source: "named"; name: string; label: string; credential: Credential }
  | { id: string; source: "dedicated"; name: string; label: string; passphrase: DedicatedKeyPassphrase };

function keyBasename(key: string): string {
  const components = key.split("/").filter(Boolean);
  return components[components.length - 1] ?? key;
}

export function credentialRows(list: CredentialList, kind: CredentialKind): CredentialRow[] {
  const named: CredentialRow[] = list.credentials
    .filter((credential) => credential.kind === kind)
    .map((credential) => ({ id: `named:${credential.name}`, source: "named", name: credential.name, label: credential.name, credential }));
  if (kind !== "key_passphrase") return named;
  const dedicated: CredentialRow[] = list.dedicatedKeyPassphrases.map((passphrase) => ({
    id: `dedicated:${passphrase.key}`,
    source: "dedicated",
    name: keyBasename(passphrase.key),
    label: passphrase.key,
    passphrase,
  }));
  return [...named, ...dedicated];
}

export function rowHosts(row: CredentialRow): string[] {
  return row.source === "named" ? row.credential.hosts : row.passphrase.hosts;
}

// rowAssignmentText is the line under a row's name: where the credential is used. A key passphrase names its
// keys as well as the hosts, and says so when the hosts could not all be read from the SSH configuration.
// The names in a list are joined by listSeparator; the parts (keys, hosts) by partSeparator.
export function rowAssignmentText(row: CredentialRow, keyHostUsageComplete: boolean, t: Translate): string {
  const hosts = rowHosts(row);
  const isKeyPassphrase = row.source === "dedicated" || row.credential.kind === "key_passphrase";
  const hostsText = isKeyPassphrase && !keyHostUsageComplete
    ? t("secrets.keyHostsUnavailable")
    : hosts.length === 0 ? "" : t("secrets.rowHosts", { hosts: hosts.join(t("secrets.listSeparator")) });
  if (row.source === "dedicated") {
    return hostsText === "" ? t("secrets.dedicated") : [t("secrets.dedicated"), hostsText].join(t("secrets.partSeparator"));
  }
  if (row.credential.kind !== "key_passphrase") {
    return hosts.length === 0 ? t("secrets.rowUnassigned") : hosts.join(t("secrets.listSeparator"));
  }
  const keys = row.credential.uses;
  const keysText = keys.length === 0 ? "" : t("secrets.rowKeys", { keys: keys.join(t("secrets.listSeparator")) });
  const parts = [keysText, hostsText].filter((part) => part !== "");
  return parts.length === 0 ? t("secrets.rowUnassigned") : parts.join(t("secrets.partSeparator"));
}
