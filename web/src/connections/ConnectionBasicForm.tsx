import { useEffect, useMemo, useState, type FormEvent } from "react";
import type { Problem } from "../api/client";
import {
  type HostDetail,
  type UpdateConnectionPassword,
  type UpdateConnectionRequest,
} from "../api/config";
import {
  integrationsApi,
  type Credential,
  type IntegrationsApi,
  type PasswordEligibility,
  type PasswordVaultStatus,
} from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { keysApi, selectablePrivateKeys, type KeyItem, type KeysApi } from "../keys/api";
import { control, hintText, sectionHeading } from "../ui/form";
import { PasswordField } from "../ui/PasswordField";
import { Button, Card, Notice, Row } from "../ui/surface";
import { deriveBasicField, type BasicFieldState, type BasicKeyword } from "./basicFields";
import { formatValues } from "./values";

type PasswordAction = UpdateConnectionPassword["kind"];

type ConnectionBasicFormProps = {
  detail: HostDetail;
  problem: Problem | null;
  onSave: (request: UpdateConnectionRequest) => Promise<void>;
  keys?: Pick<KeysApi, "inventory">;
  secrets?: Pick<
    IntegrationsApi,
    "passwordVault" | "credentials" | "passwordEligibility" | "initialiseVault" | "unlockVault"
  >;
};

type DraftField = {
  state: BasicFieldState;
  value: string;
  inherit: boolean;
};

const hostPattern = /^[A-Za-z0-9]([A-Za-z0-9._:-]*[A-Za-z0-9])?$/;

function sameKeyword(left: string, right: string): boolean {
  return left.toLocaleLowerCase() === right.toLocaleLowerCase();
}

function initialDraft(detail: HostDetail, keyword: BasicKeyword): DraftField {
  const state = deriveBasicField(detail, keyword);
  return { state, value: state.value, inherit: false };
}

function directIdentityFields(detail: HostDetail) {
  return detail.form.fields.filter((field) => sameKeyword(field.keyword, "IdentityFile"));
}

function keyConfigValue(key: KeyItem): string {
  return `~/.ssh/${key.relativePath}`;
}

function sourceText(field: BasicFieldState, t: ReturnType<typeof useTranslate>): string {
  if (field.origin === "direct") return t("conn.basicThisConnection");
  if (field.origin === "default") return t("conn.basicSSHDefault");
  if (field.origin === "complex") return t("conn.basicReadOnlyAdvanced");
  const path = field.source?.path ?? field.source?.absolute ?? "";
  return t("conn.basicInheritedFrom", { path, line: field.source?.line ?? 0 });
}

export function ConnectionBasicForm({
  detail,
  problem,
  onSave,
  keys = keysApi,
  secrets = integrationsApi,
}: ConnectionBasicFormProps) {
  const t = useTranslate();
  const identity = detail.form.entry.identity;
  const resetKey = `${identity.path}\u0000${identity.alias}\u0000${detail.file.contents}`;
  const initial = useMemo(() => ({
    hostName: initialDraft(detail, "HostName"),
    user: initialDraft(detail, "User"),
    port: initialDraft(detail, "Port"),
  }), [detail]);
  const [hostName, setHostName] = useState(initial.hostName);
  const [user, setUser] = useState(initial.user);
  const [port, setPort] = useState(initial.port);
  const [privateKeys, setPrivateKeys] = useState<KeyItem[]>([]);
  const [selectedKey, setSelectedKey] = useState("");
  const [initialKey, setInitialKey] = useState("");
  const [keyState, setKeyState] = useState<"loading" | "editable" | "custom" | "complex">("loading");
  const [customKey, setCustomKey] = useState("");
  const [vault, setVault] = useState<PasswordVaultStatus | null>(null);
  const [eligibility, setEligibility] = useState<PasswordEligibility | null>(null);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [assigned, setAssigned] = useState(false);
  const [assignedCredential, setAssignedCredential] = useState("");
  const [passwordAction, setPasswordAction] = useState<PasswordAction>("unchanged");
  const [password, setPassword] = useState("");
  const [savedCredential, setSavedCredential] = useState("");
  const [newCredential, setNewCredential] = useState("");
  const [newSharedPassword, setNewSharedPassword] = useState("");
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [masterPassword, setMasterPassword] = useState("");
  const [masterConfirmation, setMasterConfirmation] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [vaultBusy, setVaultBusy] = useState(false);
  const [localError, setLocalError] = useState("");

  function clearSecrets() {
    setPassword("");
    setNewSharedPassword("");
    setMasterPassword("");
    setMasterConfirmation("");
  }

  function applyCredentialState(status: PasswordVaultStatus, listed: Credential[]) {
    const passwordCredentials = listed.filter((credential) => credential.kind === "password");
    const reusable = passwordCredentials.find((credential) => credential.uses.includes(identity.alias));
    setCredentials(passwordCredentials);
    setSavedCredential((current) =>
      passwordCredentials.some((credential) => credential.name === current)
        ? current
        : passwordCredentials[0]?.name ?? "",
    );
    setAssigned(status.aliases.includes(identity.alias));
    setAssignedCredential(reusable?.name ?? "");
  }

  useEffect(() => {
    setHostName(initial.hostName);
    setUser(initial.user);
    setPort(initial.port);
    setPasswordAction("unchanged");
    setConfirmRemove(false);
    setNewCredential("");
    clearSecrets();
    setLocalError("");
    setLoading(true);
    setKeyState("loading");

    let active = true;
    void Promise.all([
      keys.inventory(),
      secrets.passwordVault(),
      secrets.passwordEligibility(identity.alias),
    ]).then(async ([inventory, status, nextEligibility]) => {
      const listed = status.unlocked ? (await secrets.credentials()).credentials : [];
      if (!active) return;

      const identities = selectablePrivateKeys(inventory);
      const direct = directIdentityFields(detail);
      setPrivateKeys(identities);
      if (direct.length > 1) {
        setKeyState("complex");
        setSelectedKey("");
        setInitialKey("");
      } else if (direct.length === 1) {
        const configured = formatValues(direct[0]!.values);
        const matched = identities.find((candidate) => keyConfigValue(candidate) === configured);
        if (matched === undefined) {
          setKeyState("custom");
          setCustomKey(configured);
          setSelectedKey("__custom__");
          setInitialKey("__custom__");
        } else {
          setKeyState("editable");
          setSelectedKey(matched.id);
          setInitialKey(matched.id);
        }
      } else {
        setKeyState("editable");
        setSelectedKey("");
        setInitialKey("");
      }
      setVault(status);
      setEligibility(nextEligibility);
      applyCredentialState(status, listed);
      setLoading(false);
    }).catch(() => {
      if (!active) return;
      clearSecrets();
      setLocalError(t("conn.basicOptionsFailed"));
      setLoading(false);
    });
    return () => {
      active = false;
      clearSecrets();
    };
    // resetKey deliberately represents the server snapshot this draft belongs to.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey, keys, secrets, t]);

  function updateField(setter: (value: DraftField) => void, current: DraftField, value: string) {
    setter({ ...current, value, inherit: false });
    setLocalError("");
  }

  function stringChange(field: DraftField, allowEmpty: boolean) {
    if (!field.state.editable || field.state.origin === "complex") return undefined;
    if (field.inherit || (allowEmpty && field.value === "" && field.state.origin === "direct")) {
      return field.state.origin === "direct" ? { action: "inherit" as const } : undefined;
    }
    if (field.value === field.state.value) return undefined;
    return { action: "set" as const, value: field.value };
  }

  const hostNameChange = stringChange(hostName, false);
  const userChange = stringChange(user, true);
  const portChange = port.inherit
    ? port.state.origin === "direct" ? { action: "inherit" as const } : undefined
    : port.value === port.state.value
      ? undefined
      : { action: "set" as const, value: Number(port.value) };
  const identityFileChange = keyState !== "editable" || selectedKey === initialKey
    ? undefined
    : selectedKey === ""
      ? { action: "inherit" as const }
      : { action: "set" as const, keyId: selectedKey };

  const hostError = hostName.inherit
    ? ""
    : hostName.value === ""
      ? t("conn.createHostRequired")
      : hostName.value.length <= 255 && hostPattern.test(hostName.value)
        ? ""
        : t("conn.createHostInvalid");
  const userError = user.value !== "" && /[\s\p{Cc}]/u.test(user.value) ? t("conn.createUserInvalid") : "";
  const parsedPort = Number(port.value);
  const portError = port.inherit || (/^\d+$/.test(port.value) && parsedPort >= 1 && parsedPort <= 65535)
    ? ""
    : t("conn.createPortInvalid");

  function effectivePassword(): UpdateConnectionPassword {
    switch (passwordAction) {
      case "dedicated_password":
        return password === "" ? { kind: "unchanged" } : { kind: "dedicated_password", password };
      case "saved_password":
        return savedCredential === "" ? { kind: "unchanged" } : { kind: "saved_password", credential: savedCredential };
      case "new_shared_password":
        return newCredential === "" || newSharedPassword === ""
          ? { kind: "unchanged" }
          : { kind: "new_shared_password", credential: newCredential, password: newSharedPassword };
      case "remove":
        return confirmRemove && assigned ? { kind: "remove" } : { kind: "unchanged" };
      case "unchanged":
        return { kind: "unchanged" };
    }
  }

  const passwordChange = effectivePassword();
  const changesPassword = passwordChange.kind !== "unchanged";
  const passwordAllowed = passwordChange.kind === "remove" || passwordChange.kind === "unchanged" || eligibility?.storable === true;
  const dirty = hostNameChange !== undefined || userChange !== undefined || portChange !== undefined ||
    identityFileChange !== undefined || changesPassword;
  const canSave = !loading && !busy && vault?.unlocked === true && dirty &&
    hostError === "" && userError === "" && portError === "" && passwordAllowed;

  function choosePasswordAction(action: PasswordAction) {
    clearSecrets();
    setConfirmRemove(false);
    setPasswordAction(action);
    setLocalError("");
  }

  async function openVault() {
    if (vault === null) return;
    setVaultBusy(true);
    setLocalError("");
    try {
      const status = vault.exists
        ? await secrets.unlockVault(masterPassword)
        : await secrets.initialiseVault(masterPassword);
      const listed = status.unlocked ? (await secrets.credentials()).credentials : [];
      setVault(status);
      applyCredentialState(status, listed);
      clearSecrets();
    } catch {
      clearSecrets();
      setLocalError(t(vault.exists ? "conn.createUnlockFailed" : "conn.createVaultFailed"));
    } finally {
      setVaultBusy(false);
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!canSave) return;
    const request: UpdateConnectionRequest = {
      identity,
      base: detail.file.contents,
      password: passwordChange,
    };
    if (hostNameChange !== undefined) request.hostName = hostNameChange;
    if (userChange !== undefined) request.user = userChange;
    if (portChange !== undefined) request.port = portChange;
    if (identityFileChange !== undefined) request.identityFile = identityFileChange;

    setBusy(true);
    setLocalError("");
    try {
      await onSave(request);
      clearSecrets();
    } catch {
      clearSecrets();
      setLocalError(t("conn.basicSaveFailed"));
    } finally {
      setBusy(false);
    }
  }

  const minimum = vault?.minPassphraseLength ?? 12;
  const canOpenVault = vault !== null && masterPassword.length >= minimum &&
    (vault.exists || masterConfirmation === masterPassword);
  const passwordBlockers = eligibility?.blockers ?? [];
  const passwordWarnings = eligibility?.warnings ?? [];

  return (
    <form className="flex flex-col gap-4" onSubmit={(event) => void submit(event)}>
      {problem === null ? null : <Notice tone="danger">{problem.detail ?? problem.message}</Notice>}
      {localError === "" ? null : <Notice tone="danger">{localError}</Notice>}

      <section className="flex flex-col gap-2" aria-labelledby="basic-connection-heading">
        <h3 id="basic-connection-heading" className={sectionHeading}>{t("conn.basicConnection")}</h3>
        <Card>
          <Row
            label={t("conn.basicHostName")}
            hint={sourceText(hostName.state, t)}
            warning={hostName.state.origin === "complex" ? t("conn.basicComplex", { keyword: "HostName" }) : hostError || undefined}
            action={hostName.state.origin === "direct" ? (
              <Button onClick={() => setHostName({ ...hostName, inherit: !hostName.inherit })}>
                {hostName.inherit ? t("conn.basicKeepDirect") : t("conn.basicUseInheritedHost")}
              </Button>
            ) : undefined}
          >
            <input
              aria-label={t("conn.basicHostName")}
              value={hostName.value}
              disabled={!hostName.state.editable || hostName.state.origin === "complex"}
              onChange={(event) => updateField(setHostName, hostName, event.target.value)}
              className={control}
            />
          </Row>
          <Row
            label={t("conn.basicUser")}
            hint={sourceText(user.state, t)}
            warning={user.state.origin === "complex" ? t("conn.basicComplex", { keyword: "User" }) : userError || undefined}
            action={user.state.origin === "direct" ? (
              <Button onClick={() => setUser({ ...user, inherit: !user.inherit })}>
                {user.inherit ? t("conn.basicKeepDirect") : t("conn.basicUseInheritedUser")}
              </Button>
            ) : undefined}
          >
            <input
              aria-label={t("conn.basicUser")}
              value={user.value}
              disabled={!user.state.editable || user.state.origin === "complex"}
              onChange={(event) => updateField(setUser, user, event.target.value)}
              className={control}
            />
          </Row>
          <Row
            label={t("conn.basicPort")}
            hint={sourceText(port.state, t)}
            warning={port.state.origin === "complex" ? t("conn.basicComplex", { keyword: "Port" }) : portError || undefined}
            action={port.state.origin === "direct" ? (
              <Button onClick={() => setPort({ ...port, inherit: !port.inherit })}>
                {port.inherit ? t("conn.basicKeepDirect") : t("conn.basicUseInheritedPort")}
              </Button>
            ) : undefined}
          >
            <input
              aria-label={t("conn.basicPort")}
              type="number"
              min={1}
              max={65535}
              value={port.value}
              disabled={!port.state.editable || port.state.origin === "complex"}
              onChange={(event) => updateField(setPort, port, event.target.value)}
              className={control}
            />
          </Row>
        </Card>
      </section>

      <section className="flex flex-col gap-2" aria-labelledby="basic-auth-heading">
        <h3 id="basic-auth-heading" className={sectionHeading}>{t("conn.basicAuthentication")}</h3>
        <Card>
          <Row
            label={t("conn.basicPrivateKey")}
            hint={keyState === "custom"
              ? t("conn.basicCustomKey", { path: customKey })
              : keyState === "complex"
                ? t("conn.basicComplexKey")
                : t("conn.basicKeyIndependent")}
          >
            <select
              aria-label={t("conn.basicPrivateKey")}
              value={selectedKey}
              disabled={loading || keyState === "custom" || keyState === "complex"}
              onChange={(event) => setSelectedKey(event.target.value)}
              className={control}
            >
              <option value="">{t("conn.basicAgentOrInherited")}</option>
              {keyState === "custom" ? <option value="__custom__">{customKey}</option> : null}
              {privateKeys.map((key) => (
                <option key={key.id} value={key.id}>
                  {key.relativePath}{key.fingerprint === "" ? "" : ` — ${key.fingerprint}`}
                </option>
              ))}
            </select>
          </Row>

          <div className="border-t border-hairline px-3 py-3">
            <div className="flex flex-col gap-3">
              <div>
                <p className="text-sm text-ink-muted">{t("conn.basicStoredPassword")}</p>
                {loading ? <p className={hintText}>{t("conn.createLoadingOptions")}</p> : null}
                {vault?.unlocked === true ? (
                  <p className={hintText}>
                    {assigned
                      ? assignedCredential === ""
                        ? t("conn.basicAssignedDedicated")
                        : t("conn.basicAssignedNamed", { name: assignedCredential })
                      : t("conn.basicNoPassword")}
                  </p>
                ) : null}
              </div>

              {vault !== null && !vault.unlocked ? (
                <div className="flex flex-col gap-3 rounded-lg border border-notice-line bg-notice p-3">
                  <p className="text-sm text-notice-ink">
                    {t(vault.exists ? "conn.basicVaultLocked" : "conn.basicVaultMissing")}
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

              {vault?.unlocked === true ? (
                <>
                  <label className="flex flex-col gap-1">
                    <span className="text-xs font-medium tracking-wide text-ink-muted">{t("conn.basicPasswordAction")}</span>
                    <select
                      aria-label={t("conn.basicPasswordAction")}
                      value={passwordAction}
                      onChange={(event) => choosePasswordAction(event.target.value as PasswordAction)}
                      className={control}
                    >
                      <option value="unchanged">{t("conn.basicPasswordUnchanged")}</option>
                      <option value="dedicated_password">{t(assigned ? "conn.basicReplaceDedicated" : "conn.createDedicatedPassword")}</option>
                      <option value="saved_password">{t("conn.createSavedPassword")}</option>
                      <option value="new_shared_password">{t("conn.createNewSharedPassword")}</option>
                      {assigned ? <option value="remove">{t("conn.basicRemovePassword")}</option> : null}
                    </select>
                  </label>

                  {passwordAction === "dedicated_password" ? (
                    <PasswordField
                      label={t("conn.createConnectionPassword")}
                      value={password}
                      onChange={setPassword}
                      hint={t("conn.basicEmptyPasswordUnchanged")}
                    />
                  ) : passwordAction === "saved_password" ? (
                    <label className="flex flex-col gap-1">
                      <span className="text-xs font-medium tracking-wide text-ink-muted">{t("conn.createChooseSavedPassword")}</span>
                      <select value={savedCredential} onChange={(event) => setSavedCredential(event.target.value)} className={control}>
                        {credentials.length === 0 ? <option value="">{t("conn.createNoSavedPasswords")}</option> : null}
                        {credentials.map((credential) => <option key={credential.name} value={credential.name}>{credential.name}</option>)}
                      </select>
                    </label>
                  ) : passwordAction === "new_shared_password" ? (
                    <div className="grid gap-3 sm:grid-cols-2">
                      <label className="flex flex-col gap-1">
                        <span className="text-xs font-medium tracking-wide text-ink-muted">{t("conn.createSavedPasswordName")}</span>
                        <input value={newCredential} onChange={(event) => setNewCredential(event.target.value)} className={control} />
                      </label>
                      <PasswordField label={t("conn.createNewPassword")} value={newSharedPassword} onChange={setNewSharedPassword} />
                    </div>
                  ) : passwordAction === "remove" ? (
                    <label className="flex items-start gap-2 text-sm text-danger">
                      <input
                        type="checkbox"
                        checked={confirmRemove}
                        onChange={(event) => setConfirmRemove(event.target.checked)}
                        className="mt-0.5 accent-accent"
                      />
                      <span>{t("conn.basicConfirmRemove")}</span>
                    </label>
                  ) : null}

                  {passwordBlockers.map((blocker, index) => (
                    <Notice key={`${blocker.code}-${index}`} tone="danger">{blocker.detail ?? blocker.code}</Notice>
                  ))}
                  {passwordWarnings.map((warning, index) => (
                    <Notice key={`${warning.code}-${index}`}>{warning.detail ?? warning.code}</Notice>
                  ))}
                </>
              ) : null}
            </div>
          </div>
        </Card>
      </section>

      <div className="flex items-center gap-3">
        {!dirty ? <p className={`grow ${hintText}`}>{t("conn.basicNothingChanged")}</p> :
          vault?.unlocked !== true ? <p className={`grow ${hintText}`}>{t("conn.basicNeedVault")}</p> :
            !passwordAllowed ? <p className={`grow ${hintText}`}>{t("conn.basicPasswordBlocked")}</p> : <span className="grow" />}
        <Button type="submit" kind="primary" disabled={!canSave}>
          {busy ? t("conn.basicSaving") : t("conn.basicSave")}
        </Button>
      </div>
    </form>
  );
}
