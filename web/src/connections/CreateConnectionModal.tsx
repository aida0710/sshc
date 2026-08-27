import { useEffect, useState, type FormEvent } from "react";
import { failureCode } from "../api/client";
import {
  configApi,
  type CreateConnectionAuthentication,
  type CreateConnectionRequest,
  type CreateConnectionResponse,
  type Overview,
} from "../api/config";
import {
  integrationsApi,
  type Credential,
  type IntegrationsApi,
  type PasswordVaultStatus,
} from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { keysApi, selectablePrivateKeys, type KeyItem, type KeysApi } from "../keys/api";
import { control, Field, fieldLabel, hintText, sectionHeading } from "../ui/form";
import { PasswordField } from "../ui/PasswordField";
import { isValidHostName } from "../rules/rules";
import { Button, Notice } from "../ui/surface";
import { Icon } from "../ui/icons";

type AuthenticationKind = CreateConnectionAuthentication["kind"];

export type CreateConnectionDraft = {
  alias: string;
  group: string;
  hostName: string;
  user: string;
  port: string;
  authentication: AuthenticationKind;
  savedCredential: string;
  newCredential: string;
  keyID: string;
};

export type CreationPrerequisite = "Groups" | "Keys";

type CreateConnectionModalProps = {
  groups: Overview["groups"];
  config?: Pick<typeof configApi, "createConnection">;
  keys?: Pick<KeysApi, "inventory">;
  secrets?: Pick<
    IntegrationsApi,
    "passwordVault" | "credentials" | "initialiseVault" | "unlockVault"
  >;
  initialDraft?: CreateConnectionDraft | undefined;
  onOpenPrerequisite?: (section: CreationPrerequisite, draft: CreateConnectionDraft) => void;
  onClose: () => void;
  onCreated: (result: CreateConnectionResponse) => void;
};

type TouchedField = "alias" | "hostName" | "user" | "port";

const aliasPattern = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/;
function optional(value: string): string | undefined {
  return value === "" ? undefined : value;
}

export function CreateConnectionModal({
  groups,
  config = configApi,
  keys = keysApi,
  secrets = integrationsApi,
  initialDraft,
  onOpenPrerequisite,
  onClose,
  onCreated,
}: CreateConnectionModalProps) {
  const t = useTranslate();
  const [alias, setAlias] = useState(initialDraft?.alias ?? "");
  const [group, setGroup] = useState(initialDraft?.group ?? "");
  const [hostName, setHostName] = useState(initialDraft?.hostName ?? "");
  const [user, setUser] = useState(initialDraft?.user ?? "");
  const [port, setPort] = useState(initialDraft?.port ?? "");
  const [authentication, setAuthentication] = useState<AuthenticationKind>(
    initialDraft?.authentication ?? "dedicated_password",
  );
  const [dedicatedPassword, setDedicatedPassword] = useState("");
  const [savedCredential, setSavedCredential] = useState(initialDraft?.savedCredential ?? "");
  const [newCredential, setNewCredential] = useState(initialDraft?.newCredential ?? "");
  const [newSharedPassword, setNewSharedPassword] = useState("");
  const [keyID, setKeyID] = useState(initialDraft?.keyID ?? "");
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

  function clearSecrets() {
    setDedicatedPassword("");
    setNewSharedPassword("");
    setMasterPassword("");
    setMasterConfirmation("");
  }

  function close() {
    clearSecrets();
    setError("");
    onClose();
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
        setKeyID((current) => identities.some((identity) => identity.id === current) ? current : identities[0]?.id ?? "");
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

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== "Escape" || busy || vaultBusy) return;
      event.preventDefault();
      close();
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  });

  const aliasError = alias === ""
    ? t("conn.createAliasRequired")
    : aliasPattern.test(alias)
      ? ""
      : t("conn.createAliasInvalid");
  const hostError = hostName === ""
    ? t("conn.createHostRequired")
    : isValidHostName(hostName)
      ? ""
      : t("conn.createHostInvalid");
  const userError = user !== "" && /[\s\p{Cc}]/u.test(user) ? t("conn.createUserInvalid") : "";
  const parsedPort = Number(port);
  const portError = port !== "" && (!/^\d+$/.test(port) || parsedPort < 1 || parsedPort > 65535)
    ? t("conn.createPortInvalid")
    : "";

  const authenticationReady = (() => {
    switch (authentication) {
      case "dedicated_password": return dedicatedPassword !== "";
      case "saved_password": return savedCredential !== "";
      case "new_shared_password": return newCredential !== "" && newSharedPassword !== "";
      case "identity_file": return keyID !== "";
    }
  })();
  const vaultReady = authentication === "identity_file" || vault?.unlocked === true;
  const canSubmit = !loading && !busy && vaultReady &&
    aliasError === "" && hostError === "" && userError === "" && portError === "" && authenticationReady;

  function chooseAuthentication(kind: AuthenticationKind) {
    clearSecrets();
    setError("");
    setAuthentication(kind);
  }

  function safeDraft(): CreateConnectionDraft {
    return {
      alias,
      group,
      hostName,
      user,
      port,
      authentication,
      savedCredential,
      newCredential,
      keyID,
    };
  }

  function openPrerequisite(section: CreationPrerequisite) {
    const draft = safeDraft();
    clearSecrets();
    setError("");
    onOpenPrerequisite?.(section, draft);
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
      clearSecrets();
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
        return { kind: authentication, keyId: keyID };
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setTouched(new Set(["alias", "hostName", "user", "port"]));
    if (!canSubmit) return;
    setBusy(true);
    setError("");
    const request: CreateConnectionRequest = {
      alias,
      group,
      hostName,
      authentication: requestAuthentication(),
    };
    const selectedUser = optional(user);
    if (selectedUser !== undefined) request.user = selectedUser;
    if (port !== "") request.port = parsedPort;

    try {
      const result = await config.createConnection(request);
      clearSecrets();
      onCreated(result);
    } catch (caught) {
      clearSecrets();
      const code = failureCode(caught);
      switch (code) {
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

  const minimum = vault?.minPassphraseLength ?? 12;
  const canOpenVault = vault !== null && masterPassword.length >= minimum &&
    (vault.exists || masterConfirmation === masterPassword);
  const disabledReason = (() => {
    if (canSubmit || busy) return "";
    if (loading) return t("conn.createLoadingOptions");
    if (aliasError !== "") return touched.has("alias") ? "" : aliasError;
    if (hostError !== "") return touched.has("hostName") ? "" : hostError;
    if (userError !== "") return touched.has("user") ? "" : userError;
    if (portError !== "") return touched.has("port") ? "" : portError;
    if (!vaultReady) return t("conn.createNeedVault");
    switch (authentication) {
      case "dedicated_password": return t("conn.createNeedConnectionPassword");
      case "saved_password": return t("conn.createNeedSavedPassword");
      case "new_shared_password":
        return newCredential === "" ? t("conn.createNeedSavedPasswordName") : t("conn.createNeedNewPassword");
      case "identity_file": return t("conn.createNeedPrivateKey");
    }
  })();

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-canvas/80 p-4 backdrop-blur-sm">
      <section
        role="dialog"
        aria-modal="true"
        aria-labelledby="create-connection-heading"
        className="sshc-card flex max-h-full w-full max-w-2xl flex-col overflow-hidden rounded-lg bg-card shadow-xl"
      >
        <div className="border-b border-line px-5 py-5">
          <div className="flex items-start gap-3">
            <span aria-hidden="true" className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-select-fill text-accent">
              <Icon name="plus" className="size-5" />
            </span>
            <div>
              <h2 id="create-connection-heading" className="text-lg font-semibold tracking-tight text-ink">{t("conn.createTitle")}</h2>
              <p className={`mt-1 ${hintText}`}>{t("conn.createDescription")}</p>
            </div>
          </div>
        </div>

        <form className="flex min-h-0 flex-col" onSubmit={(event) => void submit(event)}>
          <div className="flex min-h-0 flex-col gap-6 overflow-y-auto p-5">
            {error === "" ? null : <Notice tone="danger">{error}</Notice>}
            <section className="flex flex-col gap-3" aria-labelledby="create-connection-section">
              <div className="flex flex-wrap items-end justify-between gap-3">
                <h3 id="create-connection-section" className={sectionHeading}>{t("conn.createConnectionSection")}</h3>
                {alias === "" || hostName === "" ? null : (
                  <p className="rounded-md bg-tree px-2.5 py-1.5 font-mono text-xs text-ink-muted">
                    {`${alias}  ${user === "" ? "" : `${user}@`}${hostName}:${port || "22"}`}
                  </p>
                )}
              </div>
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="sm:col-span-2">
                  <Field label={t("conn.createNameRequired")}>
                    <input
                      id="create-connection-name"
                      aria-label={t("conn.createName")}
                      required
                      autoFocus
                      value={alias}
                      onChange={(event) => setAlias(event.target.value)}
                      onBlur={() => setTouched((current) => new Set(current).add("alias"))}
                      aria-invalid={touched.has("alias") && aliasError !== ""}
                      className={control}
                    />
                  </Field>
                  {touched.has("alias") && aliasError !== "" ? <p className="mt-1 text-xs text-danger">{aliasError}</p> : null}
                </div>
                <div>
                  <Field label={t("conn.createGroup")}>
                    <select id="create-connection-group" value={group} onChange={(event) => setGroup(event.target.value)} className={control}>
                      <option value="">{t("conn.createNoGroup")}</option>
                      {groups.map((candidate) => <option key={candidate.name} value={candidate.name}>{candidate.name}</option>)}
                    </select>
                  </Field>
                  <Button className="mt-2" onClick={() => openPrerequisite("Groups")}>{t("conn.createManageGroups")}</Button>
                </div>
                <div>
                  <Field label={t("conn.createHostNameRequired")}>
                    <input
                      id="create-connection-hostname"
                      aria-label={t("conn.createHostName")}
                      required
                      value={hostName}
                      onChange={(event) => setHostName(event.target.value)}
                      onBlur={() => setTouched((current) => new Set(current).add("hostName"))}
                      aria-invalid={touched.has("hostName") && hostError !== ""}
                      className={control}
                    />
                  </Field>
                  {touched.has("hostName") && hostError !== "" ? <p className="mt-1 text-xs text-danger">{hostError}</p> : null}
                </div>
                <div>
                  <Field label={t("conn.createUser")}>
                    <input
                      id="create-connection-user"
                      value={user}
                      onChange={(event) => setUser(event.target.value)}
                      onBlur={() => setTouched((current) => new Set(current).add("user"))}
                      aria-invalid={touched.has("user") && userError !== ""}
                      className={control}
                    />
                  </Field>
                  {touched.has("user") && userError !== "" ? <p className="mt-1 text-xs text-danger">{userError}</p> : null}
                </div>
                <div>
                  <Field label={t("conn.createPort")} hint={t("conn.createPortHint")}>
                    <input
                      id="create-connection-port"
                      type="number"
                      min={1}
                      max={65535}
                      value={port}
                      onChange={(event) => setPort(event.target.value)}
                      onBlur={() => setTouched((current) => new Set(current).add("port"))}
                      aria-invalid={touched.has("port") && portError !== ""}
                      className={control}
                    />
                  </Field>
                  {touched.has("port") && portError !== "" ? <p className="mt-1 text-xs text-danger">{portError}</p> : null}
                </div>
              </div>
            </section>

            <section className="flex flex-col gap-3 border-t border-line pt-5" aria-labelledby="create-auth-section">
              <h3 id="create-auth-section" className={sectionHeading}>{t("conn.createAuthenticationSection")}</h3>
              {loading ? <p className={hintText}>{t("conn.createLoadingOptions")}</p> : null}
              {vault !== null && !vault.unlocked && authentication !== "identity_file" ? (
                <div className="flex flex-col gap-3 rounded-lg border border-notice-line bg-notice p-3">
                  <p className="text-sm text-notice-ink">
                    {t(vault.exists ? "conn.createVaultLocked" : "conn.createVaultMissing")}
                  </p>
                  <PasswordField label={t("conn.createMasterPassword")} value={masterPassword} onChange={setMasterPassword} />
                  {vault.exists ? null : (
                    <PasswordField
                      label={t("conn.createConfirmMaster")}
                      value={masterConfirmation}
                      onChange={setMasterConfirmation}
                    />
                  )}
                  <Button kind="primary" disabled={vaultBusy || !canOpenVault} onClick={() => void openVault()}>
                    {t(vault.exists ? "conn.createUnlockVault" : "conn.createInitialiseVault")}
                  </Button>
                </div>
              ) : null}

              <fieldset className="grid gap-2 sm:grid-cols-2" disabled={loading}>
                <legend className={fieldLabel}>{t("conn.createAuthenticationMethod")}</legend>
                {([
                  ["identity_file", "conn.createIdentityFile"],
                  ["dedicated_password", "conn.createDedicatedPassword"],
                  ["saved_password", "conn.createSavedPassword"],
                  ["new_shared_password", "conn.createNewSharedPassword"],
                ] as const).map(([kind, label]) => (
                  <label key={kind} className="flex cursor-pointer items-center gap-2 rounded-lg border border-line bg-card p-3 text-sm text-ink transition-colors hover:bg-select-fill has-[:checked]:border-accent has-[:checked]:bg-select-fill">
                    <input
                      type="radio"
                      name="create-authentication"
                      value={kind}
                      checked={authentication === kind}
                      onChange={() => chooseAuthentication(kind)}
                      className="accent-accent"
                    />
                    <span>{t(label)}</span>
                  </label>
                ))}
              </fieldset>

              {!loading && privateKeys.length === 0 ? (
                <div>
                  <p className={hintText}>{t("conn.createNoPrivateKeysHint")}</p>
                  <Button className="mt-2" onClick={() => openPrerequisite("Keys")}>{t("conn.createCreatePrivateKey")}</Button>
                </div>
              ) : null}

              {authentication === "identity_file" ? (
                <Field label={t("conn.createPrivateKey")}>
                  <select value={keyID} onChange={(event) => setKeyID(event.target.value)} className={control}>
                    {privateKeys.length === 0 ? <option value="">{t("conn.createNoPrivateKeys")}</option> : null}
                    {privateKeys.map((key) => (
                      <option key={key.id} value={key.id}>{key.relativePath}{key.fingerprint === "" ? "" : ` · ${key.fingerprint}`}</option>
                    ))}
                  </select>
                </Field>
              ) : vault?.unlocked !== true ? null : authentication === "dedicated_password" ? (
                <PasswordField
                  label={t("conn.createConnectionPassword")}
                  value={dedicatedPassword}
                  onChange={setDedicatedPassword}
                  hint={t("conn.createDedicatedHint")}
                />
              ) : authentication === "saved_password" ? (
                <Field label={t("conn.createChooseSavedPassword")} hint={t("conn.createSavedHint")}>
                  <select value={savedCredential} onChange={(event) => setSavedCredential(event.target.value)} className={control}>
                    {credentials.length === 0 ? <option value="">{t("conn.createNoSavedPasswords")}</option> : null}
                    {credentials.map((credential) => (
                      <option key={credential.name} value={credential.name}>{credential.name}</option>
                    ))}
                  </select>
                </Field>
              ) : authentication === "new_shared_password" ? (
                <div className="grid gap-3 sm:grid-cols-2">
                  <Field label={t("conn.createSavedPasswordName")}>
                    <input value={newCredential} onChange={(event) => setNewCredential(event.target.value)} className={control} />
                  </Field>
                  <PasswordField label={t("conn.createNewPassword")} value={newSharedPassword} onChange={setNewSharedPassword} />
                </div>
              ) : null}
            </section>
          </div>

          <div className="flex shrink-0 items-center gap-2 border-t border-line px-5 py-4">
            {disabledReason === "" ? <span className="grow" /> : <p className={`grow ${hintText}`}>{disabledReason}</p>}
            <Button disabled={busy || vaultBusy} onClick={close}>{t("conn.cancelCreate")}</Button>
            <Button type="submit" kind="primary" disabled={!canSubmit}>
              {busy ? t("conn.creating") : t("conn.create")}
            </Button>
          </div>
        </form>
      </section>
    </div>
  );
}
