import { useEffect, useState } from "react";
import type { HostDetail } from "../api/config";
import type { Credential } from "../api/credentials";
import type { PasswordEligibility, PasswordVaultStatus } from "../api/vault";
import { useTranslate } from "../i18n/context";
import { selectablePrivateKeys, type KeyItem, type KeysApi } from "../keys/api";
import type { ConnectionSavedState } from "./connectionSavedState";
import type { ConnectionSecretsApi } from "./secretsApi";

export type KeyOptionsStatus = "loading" | "ready" | "failed";
export type CredentialOptionsStatus = "loading" | "ready" | "locked" | "failed";

// What the vault says about one host: the credentials it could use and the
// ones it already has.
export type CredentialState = {
  passwords: Credential[];
  keyPassphrases: Credential[];
  totps: Credential[];
  assigned: boolean;
  assignedCredential: string;
  assignedTOTP: string;
};

const noCredentials: CredentialState = {
  passwords: [], keyPassphrases: [], totps: [], assigned: false, assignedCredential: "", assignedTOTP: "",
};

function credentialStateOf(alias: string, status: PasswordVaultStatus | null, listed: Credential[]): CredentialState {
  const passwords = listed.filter((credential) => credential.kind === "password");
  const totps = listed.filter((credential) => credential.kind === "totp");
  return {
    passwords,
    keyPassphrases: listed.filter((credential) => credential.kind === "key_passphrase"),
    totps,
    assigned: status?.aliases.includes(alias) ?? false,
    assignedCredential: passwords.find((credential) => credential.uses.includes(alias))?.name ?? "",
    assignedTOTP: totps.find((credential) => credential.uses.includes(alias))?.name ?? "",
  };
}

// Everything the basic form reads from the key inventory and the vault, and
// the two ways it changes them: opening the vault and re-reading after a
// save. A form is handed either the state its page already loaded, or the
// APIs to load it with.
export function useConnectionSecrets({ detail, resetKey, keys, secrets, savedState }: {
  detail: HostDetail;
  // Changing this discards everything and reads it again.
  resetKey: string;
  keys: Pick<KeysApi, "inventory">;
  secrets: ConnectionSecretsApi;
  savedState?: ConnectionSavedState | undefined;
}) {
  const t = useTranslate();
  const alias = detail.form.entry.identity.alias;
  const [privateKeys, setPrivateKeys] = useState<KeyItem[]>([]);
  const [keyOptionsStatus, setKeyOptionsStatus] = useState<KeyOptionsStatus>("loading");
  const [vault, setVault] = useState<PasswordVaultStatus | null>(null);
  const [eligibility, setEligibility] = useState<PasswordEligibility | null>(null);
  const [credentials, setCredentials] = useState<CredentialState>(noCredentials);
  const [credentialOptionsStatus, setCredentialOptionsStatus] = useState<CredentialOptionsStatus>("loading");
  const [loading, setLoading] = useState(true);
  const [vaultBusy, setVaultBusy] = useState(false);
  const [error, setError] = useState("");
  // Counts loads, so the form can reset its draft to what was just read.
  const [generation, setGeneration] = useState(0);

  useEffect(() => {
    setError("");
    setLoading(true);
    setKeyOptionsStatus("loading");
    setCredentialOptionsStatus("loading");

    let active = true;
    if (savedState !== undefined) {
      const keysReady = savedState.keys.status === "ready";
      setPrivateKeys(savedState.keys.status === "ready" ? savedState.keys.value : []);
      setKeyOptionsStatus(keysReady ? "ready" : "failed");
      const status = savedState.vault.status === "ready" ? savedState.vault.value : null;
      setVault(status);
      setEligibility(savedState.eligibility.status === "ready" ? savedState.eligibility.value : null);
      if (status !== null && savedState.credentials.status === "ready") {
        setCredentials(credentialStateOf(alias, status, savedState.credentials.value));
        setCredentialOptionsStatus("ready");
      } else {
        setCredentials(credentialStateOf(alias, status, []));
        setCredentialOptionsStatus(savedState.credentials.status === "locked" ? "locked" : "failed");
      }
      setLoading(false);
      setGeneration((current) => current + 1);
      return () => { active = false; };
    }

    void Promise.all([
      keys.inventory(),
      secrets.passwordVault(),
      secrets.passwordEligibility(alias),
    ]).then(async ([inventory, status, nextEligibility]) => {
      const listed = status.unlocked ? (await secrets.credentials()).credentials : [];
      if (!active) return;
      setPrivateKeys(selectablePrivateKeys(inventory));
      setKeyOptionsStatus("ready");
      setVault(status);
      setEligibility(nextEligibility);
      setCredentials(credentialStateOf(alias, status, listed));
      setCredentialOptionsStatus(status.unlocked ? "ready" : "locked");
      setLoading(false);
      setGeneration((current) => current + 1);
    }).catch(() => {
      if (!active) return;
      setError(t("conn.basicOptionsFailed"));
      setKeyOptionsStatus("failed");
      setCredentialOptionsStatus("failed");
      setLoading(false);
      setGeneration((current) => current + 1);
    });
    return () => { active = false; };
    // `alias` is part of resetKey; listing it as well would refetch twice.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey, keys, secrets, t, savedState]);

  // Resolves to true when the vault is open afterwards.
  async function openVault(masterPassword: string): Promise<boolean> {
    if (vault === null) return false;
    setVaultBusy(true);
    setError("");
    try {
      const status = vault.exists
        ? await secrets.unlockVault(masterPassword)
        : await secrets.initialiseVault(masterPassword);
      const [listed, nextEligibility] = status.unlocked
        ? await Promise.all([
            secrets.credentials().then((response) => response.credentials),
            secrets.passwordEligibility(alias),
          ])
        : [[], eligibility];
      setVault(status);
      setCredentials(credentialStateOf(alias, status, listed));
      setEligibility(nextEligibility);
      setCredentialOptionsStatus(status.unlocked ? "ready" : "locked");
      return status.unlocked;
    } catch {
      setError(t(vault.exists ? "conn.createUnlockFailed" : "conn.createVaultFailed"));
      return false;
    } finally {
      setVaultBusy(false);
    }
  }

  // After a save changed what the vault holds for this host.
  async function refreshCredentials(): Promise<void> {
    try {
      const status = await secrets.passwordVault();
      const listed = status.unlocked ? (await secrets.credentials()).credentials : [];
      setVault(status);
      setCredentials(credentialStateOf(alias, status, listed));
      setCredentialOptionsStatus(status.unlocked ? "ready" : "locked");
    } catch {
      setCredentialOptionsStatus("failed");
      setError(t("conn.basicRefreshFailed"));
    }
  }

  return {
    privateKeys,
    keyOptionsStatus,
    vault,
    eligibility,
    credentials,
    credentialOptionsStatus,
    loading,
    vaultBusy,
    error,
    setError,
    generation,
    openVault,
    refreshCredentials,
    unlocked: vault?.unlocked === true && credentialOptionsStatus === "ready",
  };
}

export type ConnectionSecretsModel = ReturnType<typeof useConnectionSecrets>;
