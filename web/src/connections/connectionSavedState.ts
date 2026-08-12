import type { HostDetail } from "../api/config";
import type {
  Credential,
  IntegrationsApi,
  PasswordEligibility,
  PasswordVaultStatus,
} from "../api/integrations";
import { selectablePrivateKeys, type KeyItem, type KeysApi } from "../keys/api";
import { deriveBasicField } from "./basicFields";
import { directIdentityFields, isConcreteIdentityValue } from "./authenticationPolicy";
import { formatValues } from "./values";

export type Loadable<T> =
  | { status: "loading" }
  | { status: "ready"; value: T }
  | { status: "locked" }
  | { status: "failed" };

export type ConnectionSavedState = {
  detail: HostDetail;
  keys: Loadable<KeyItem[]>;
  vault: Loadable<PasswordVaultStatus>;
  credentials: Loadable<Credential[]>;
  eligibility: Loadable<PasswordEligibility>;
};

export type ConnectionSummaryView = {
  alias: string;
  endpoint: string;
  group: string;
  privateKey:
    | { state: "none" }
    | { state: "known"; path: string; fingerprint: string; encrypted: boolean }
    | { state: "custom"; path: string }
    | { state: "complex" }
    | { state: "unavailable"; path: string };
  keyPassphrase:
    | { state: "none" | "dedicated" | "not_needed" | "locked" | "unavailable" }
    | { state: "named"; name: string };
  accountPassword:
    | { state: "none" | "dedicated" | "locked" | "unavailable" }
    | { state: "named"; name: string };
};

type SavedStateSecrets = Pick<
  IntegrationsApi,
  "passwordVault" | "credentials" | "passwordEligibility"
>;

function loaded<T>(result: PromiseSettledResult<T>): Loadable<T> {
  return result.status === "fulfilled"
    ? { status: "ready", value: result.value }
    : { status: "failed" };
}

export async function loadConnectionSavedState(
  detail: HostDetail,
  keys: Pick<KeysApi, "inventory">,
  secrets: SavedStateSecrets,
): Promise<ConnectionSavedState> {
  const alias = detail.form.entry.identity.alias;
  const [inventoryResult, vaultResult, eligibilityResult] = await Promise.allSettled([
    keys.inventory(),
    secrets.passwordVault(),
    secrets.passwordEligibility(alias),
  ]);

  const keyState: Loadable<KeyItem[]> = inventoryResult.status === "fulfilled"
    ? { status: "ready", value: selectablePrivateKeys(inventoryResult.value) }
    : { status: "failed" };
  const vaultState = loaded(vaultResult);
  const eligibilityState = loaded(eligibilityResult);
  let credentialState: Loadable<Credential[]>;
  if (vaultResult.status === "rejected") {
    credentialState = { status: "failed" };
  } else if (!vaultResult.value.unlocked) {
    credentialState = { status: "locked" };
  } else {
    try {
      credentialState = { status: "ready", value: (await secrets.credentials()).credentials };
    } catch {
      credentialState = { status: "failed" };
    }
  }

  return {
    detail,
    keys: keyState,
    vault: vaultState,
    credentials: credentialState,
    eligibility: eligibilityState,
  };
}

function directIdentityValues(detail: HostDetail): string[] {
  return directIdentityFields(detail)
    .map((field) => field.values.filter(isConcreteIdentityValue))
    .filter((values) => values.length > 0)
    .map(formatValues);
}

function configuredKey(
  detail: HostDetail,
  keys: Loadable<KeyItem[]>,
): ConnectionSummaryView["privateKey"] {
  const direct = directIdentityValues(detail);
  if (direct.length === 0) return { state: "none" };
  if (direct.length > 1) return { state: "complex" };
  const path = direct[0] ?? "";
  if (keys.status !== "ready") return { state: "unavailable", path };
  const matched = keys.value.find((key) => `~/.ssh/${key.relativePath}` === path);
  if (matched === undefined) return { state: "custom", path };
  return {
    state: "known",
    path: matched.relativePath,
    fingerprint: matched.fingerprint,
    encrypted: matched.encrypted,
  };
}

function accountPassword(
  alias: string,
  vault: Loadable<PasswordVaultStatus>,
  credentials: Loadable<Credential[]>,
): ConnectionSummaryView["accountPassword"] {
  if (vault.status === "locked" || credentials.status === "locked") return { state: "locked" };
  if (vault.status !== "ready" || credentials.status === "failed") return { state: "unavailable" };
  if (!vault.value.unlocked) return { state: "locked" };
  if (credentials.status !== "ready") return { state: "unavailable" };
  if (!vault.value.aliases.includes(alias)) return { state: "none" };
  const named = credentials.value.find(
    (credential) => credential.kind === "password" && credential.uses.includes(alias),
  );
  return named === undefined ? { state: "dedicated" } : { state: "named", name: named.name };
}

function keyPassphrase(
  key: ConnectionSummaryView["privateKey"],
  vault: Loadable<PasswordVaultStatus>,
  credentials: Loadable<Credential[]>,
): ConnectionSummaryView["keyPassphrase"] {
  if (key.state === "none") return { state: "none" };
  if (key.state !== "known") return { state: "unavailable" };
  if (!key.encrypted) return { state: "not_needed" };
  if (vault.status === "locked" || credentials.status === "locked") return { state: "locked" };
  if (vault.status !== "ready" || credentials.status === "failed") return { state: "unavailable" };
  if (!vault.value.unlocked) return { state: "locked" };
  if (credentials.status !== "ready") return { state: "unavailable" };
  if (vault.value.dedicatedKeyPassphrases.includes(key.path)) return { state: "dedicated" };
  const named = credentials.value.find(
    (credential) => credential.kind === "key_passphrase" && credential.uses.includes(key.path),
  );
  return named === undefined ? { state: "none" } : { state: "named", name: named.name };
}

export function summarizeConnection(saved: ConnectionSavedState): ConnectionSummaryView {
  const { detail } = saved;
  const alias = detail.form.entry.identity.alias;
  const hostName = deriveBasicField(detail, "HostName").value || alias;
  const user = deriveBasicField(detail, "User").value;
  const port = deriveBasicField(detail, "Port").value || "22";
  const privateKey = configuredKey(detail, saved.keys);
  return {
    alias,
    endpoint: `${user === "" ? "" : `${user}@`}${hostName}:${port}`,
    group: detail.form.entry.group ?? "",
    privateKey,
    keyPassphrase: keyPassphrase(privateKey, saved.vault, saved.credentials),
    accountPassword: accountPassword(alias, saved.vault, saved.credentials),
  };
}
