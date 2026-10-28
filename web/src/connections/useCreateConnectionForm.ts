import { useEffect, useState, type FormEvent } from "react";
import { failureCode } from "../api/client";
import {
  configApi,
  type CreateConnectionAuthentication,
  type CreateConnectionRequest,
  type CreateConnectionResponse,
} from "../api/config";
import type { Credential } from "../api/credentials";
import type { PasswordVaultStatus } from "../api/vault";
import { useTranslate } from "../i18n/context";
import { keysApi, selectablePrivateKeys, type KeyItem, type KeysApi } from "../keys/api";
import { meetsMasterPasswordMinimum } from "../secrets/masterPasswordLength";
import type { CreateConnectionAuthenticationKind, CreateConnectionDraft } from "./createConnectionDraft";
import { createConnectionFieldErrors } from "./createConnectionValidation";
import { connectionSecretsApi, type ConnectionSecretsApi } from "./secretsApi";

export type CreateConnectionFormOptions = {
  config?: Pick<typeof configApi, "createConnection">;
  keys?: Pick<KeysApi, "inventory">;
  secrets?: Pick<ConnectionSecretsApi, "passwordVault" | "credentials" | "initialiseVault" | "unlockVault">;
  initialDraft?: CreateConnectionDraft | undefined;
  onCreated: (result: CreateConnectionResponse) => void;
};

type TouchedField = "alias" | "hostName" | "user" | "port";

function optional(value: string): string | undefined {
  return value === "" ? undefined : value;
}

// useCreateConnectionForm は、新しい接続の欄の値と、認証の選択肢の読み込み、Vault を開くこと、作成の要求を持つ。
// 欄の見た目は CreateConnectionPane が持つ。
export function useCreateConnectionForm({
  config = configApi,
  keys = keysApi,
  secrets = connectionSecretsApi,
  initialDraft,
  onCreated,
}: CreateConnectionFormOptions) {
  const t = useTranslate();
  const [alias, setAlias] = useState(initialDraft?.alias ?? "");
  const [group, setGroup] = useState(initialDraft?.group ?? "");
  const [hostName, setHostName] = useState(initialDraft?.hostName ?? "");
  const [user, setUser] = useState(initialDraft?.user ?? "");
  const [port, setPort] = useState(initialDraft?.port ?? "");
  const [authentication, setAuthentication] = useState<CreateConnectionAuthenticationKind>(
    initialDraft?.authentication ?? "dedicated_password",
  );
  const [dedicatedPassword, setDedicatedPassword] = useState("");
  const [savedCredential, setSavedCredential] = useState(initialDraft?.savedCredential ?? "");
  const [newCredential, setNewCredential] = useState(initialDraft?.newCredential ?? "");
  const [newSharedPassword, setNewSharedPassword] = useState("");
  const [keyId, setKeyId] = useState(initialDraft?.keyId ?? "");
  const [privateKeys, setPrivateKeys] = useState<KeyItem[]>([]);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [vault, setVault] = useState<PasswordVaultStatus | null>(null);
  const [masterPassword, setMasterPassword] = useState("");
  const [masterConfirmation, setMasterConfirmation] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [vaultBusy, setVaultBusy] = useState(false);
  const [error, setError] = useState("");
  const [touched, setTouched] = useState<Set<TouchedField>>(() => new Set());

  // Passwords leave the form when it closes, when another authentication is chosen, when a prerequisite
  // screen takes over, and when a request lands. A refused request keeps them so that the refusal is fixed
  // without retyping, as the lock screen and the VPN profile form do.
  function clearSecrets() {
    setDedicatedPassword("");
    setNewSharedPassword("");
    setMasterPassword("");
    setMasterConfirmation("");
  }

  useEffect(() => {
    let active = true;
    void Promise.all([secrets.passwordVault(), keys.inventory()])
      .then(async ([status, inventory]) => {
        const listed = status.unlocked ? (await secrets.credentials()).credentials : [];
        if (!active) return;
        const passwordCredentials = listed.filter((credential) => credential.kind === "password");
        const identities = selectablePrivateKeys(inventory);
        setVault(status);
        setCredentials(passwordCredentials);
        setSavedCredential((current) =>
          passwordCredentials.some((credential) => credential.name === current)
            ? current
            : passwordCredentials[0]?.name ?? "",
        );
        setPrivateKeys(identities);
        setKeyId((current) => identities.some((identity) => identity.id === current) ? current : identities[0]?.id ?? "");
        if (initialDraft === undefined && identities.length > 0) setAuthentication("identity_file");
        setLoading(false);
      })
      .catch(() => {
        if (!active) return;
        setError(t("conn.createOptionsFailed"));
        setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [initialDraft, keys, secrets, t]);

  const fieldErrors = createConnectionFieldErrors(t, { alias, hostName, user, port });
  const authenticationReady = (() => {
    switch (authentication) {
      case "dedicated_password": return dedicatedPassword !== "";
      case "saved_password": return savedCredential !== "";
      case "new_shared_password": return newCredential !== "" && newSharedPassword !== "";
      case "identity_file": return keyId !== "";
    }
  })();
  const vaultReady = authentication === "identity_file" || vault?.unlocked === true;
  const fieldsValid = Object.values(fieldErrors).every((message) => message === "");
  const canSubmit = !loading && !busy && vaultReady && fieldsValid && authenticationReady;
  const canOpenVault = vault !== null && meetsMasterPasswordMinimum(masterPassword, vault.minPassphraseLength) &&
    (vault.exists || masterConfirmation === masterPassword);
  // 何かを入れたら下書きとみなし、ほかの接続を開くときや画面を離れるときに捨ててよいかを確かめる。
  const dirty = [alias, group, hostName, user, port, dedicatedPassword, newCredential, newSharedPassword].some((value) => value !== "");

  // まだ作成できない理由。欄の下の帯に出す。離れた欄の問題は、その欄の下に出すのでここでは出さない。
  const disabledReason = (() => {
    if (canSubmit || busy) return "";
    if (loading) return t("conn.createLoadingOptions");
    for (const field of ["alias", "hostName", "user", "port"] as const) {
      if (fieldErrors[field] !== "") return touched.has(field) ? "" : fieldErrors[field];
    }
    if (!vaultReady) return t("conn.createNeedVault");
    switch (authentication) {
      case "dedicated_password": return t("conn.createNeedConnectionPassword");
      case "saved_password": return t("conn.createNeedSavedPassword");
      case "new_shared_password":
        return newCredential === "" ? t("conn.createNeedSavedPasswordName") : t("conn.createNeedNewPassword");
      case "identity_file": return t("conn.createNeedPrivateKey");
    }
  })();

  function touch(field: TouchedField) {
    setTouched((current) => new Set(current).add(field));
  }

  function chooseAuthentication(kind: CreateConnectionAuthenticationKind) {
    clearSecrets();
    setError("");
    setAuthentication(kind);
  }

  function safeDraft(): CreateConnectionDraft {
    return { alias, group, hostName, user, port, authentication, savedCredential, newCredential, keyId };
  }

  async function openVault() {
    if (vault === null) return;
    setVaultBusy(true);
    setError("");
    try {
      const status = vault.exists
        ? await secrets.unlockVault(masterPassword)
        : await secrets.initialiseVault(masterPassword);
      const listed = status.unlocked ? (await secrets.credentials()).credentials : [];
      const passwordCredentials = listed.filter((credential) => credential.kind === "password");
      setVault(status);
      setCredentials(passwordCredentials);
      setSavedCredential(passwordCredentials[0]?.name ?? "");
      setMasterPassword("");
      setMasterConfirmation("");
    } catch {
      setError(t(vault.exists ? "conn.createUnlockFailed" : "conn.createVaultFailed"));
    } finally {
      setVaultBusy(false);
    }
  }

  function requestAuthentication(): CreateConnectionAuthentication {
    switch (authentication) {
      case "dedicated_password":
        return { kind: authentication, password: dedicatedPassword };
      case "saved_password":
        return { kind: authentication, credential: savedCredential };
      case "new_shared_password":
        return { kind: authentication, credential: newCredential, password: newSharedPassword };
      case "identity_file":
        return { kind: authentication, keyId };
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setTouched(new Set(["alias", "hostName", "user", "port"]));
    if (!canSubmit) return;
    setBusy(true);
    setError("");
    const request: CreateConnectionRequest = { alias, group, hostName, authentication: requestAuthentication() };
    const selectedUser = optional(user);
    if (selectedUser !== undefined) request.user = selectedUser;
    if (port !== "") request.port = Number(port);

    try {
      const result = await config.createConnection(request);
      clearSecrets();
      onCreated(result);
    } catch (caught) {
      switch (failureCode(caught)) {
        case "alias_already_declared": setError(t("conn.createAliasTaken")); break;
        case "group_not_declared": setError(t("conn.createGroupMissing")); break;
        case "identity_file_invalid": setError(t("conn.createKeyInvalid")); break;
        case "unknown_credential": setError(t("conn.createCredentialMissing")); break;
        case "connection_destination_exists": setError(t("conn.createDestinationExists")); break;
        default: setError(t("conn.createFailed"));
      }
    } finally {
      setBusy(false);
    }
  }

  return {
    values: { alias, group, hostName, user, port, authentication, dedicatedPassword, savedCredential, newCredential, newSharedPassword, keyId, masterPassword, masterConfirmation },
    setAlias, setGroup, setHostName, setUser, setPort,
    setDedicatedPassword, setSavedCredential, setNewCredential, setNewSharedPassword, setKeyId,
    setMasterPassword, setMasterConfirmation,
    privateKeys, credentials, vault,
    loading, busy, vaultBusy, error,
    fieldErrors, touched, touch,
    canSubmit, canOpenVault, dirty, disabledReason,
    chooseAuthentication, clearSecrets, safeDraft, openVault, submit,
    clearError: () => setError(""),
  };
}

export type CreateConnectionForm = ReturnType<typeof useCreateConnectionForm>;
