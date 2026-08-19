import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import type { Problem } from "../api/client";
import {
  type HostDetail,
  type UpdateConnectionKeyPassphrase,
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
import { eligibilityText } from "./eligibilityText";
import { directIdentityFields, isConcreteIdentityValue } from "./authenticationPolicy";
import { control, hintText, sectionHeading } from "../ui/form";
import { PasswordField } from "../ui/PasswordField";
import { Button, Card, Notice, Row } from "../ui/surface";
import { deriveBasicField, type BasicFieldState, type BasicKeyword } from "./basicFields";
import { formatValues } from "./values";
import { validHostNameInput } from "./hostValidation";
import type { GeneratedPrivateKeyHandoff } from "../keys/workflow";
import type { ConnectionSavedState } from "./connectionSavedState";

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
  preferredKey?: GeneratedPrivateKeyHandoff | null | undefined;
  onPreferredKeyApplied?: (() => void) | undefined;
  savedState?: ConnectionSavedState | undefined;
  onDirtyChange?: ((dirty: boolean) => void) | undefined;
  onDiscardReady?: ((discard: (() => void) | null) => void) | undefined;
  onRequestRefresh?: (() => Promise<void>) | undefined;
  disabled?: boolean | undefined;
};

type DraftField = {
  state: BasicFieldState;
  value: string;
  inherit: boolean;
};

function initialDraft(detail: HostDetail, keyword: BasicKeyword): DraftField {
  const state = deriveBasicField(detail, keyword);
  return { state, value: state.value, inherit: false };
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
  preferredKey = null,
  onPreferredKeyApplied,
  savedState,
  onDirtyChange,
  onDiscardReady,
  onRequestRefresh,
  disabled = false,
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
  const [preferredSuperseded, setPreferredSuperseded] = useState(false);
  const [keyState, setKeyState] = useState<"loading" | "editable" | "custom" | "complex">("loading");
  const [customKey, setCustomKey] = useState("");
  const [vault, setVault] = useState<PasswordVaultStatus | null>(null);
  const [eligibility, setEligibility] = useState<PasswordEligibility | null>(null);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [keyCredentials, setKeyCredentials] = useState<Credential[]>([]);
  const [assigned, setAssigned] = useState(false);
  const [assignedCredential, setAssignedCredential] = useState("");
  const [passwordAction, setPasswordAction] = useState<PasswordAction>("unchanged");
  const [password, setPassword] = useState("");
  const [savedCredential, setSavedCredential] = useState("");
  const [newCredential, setNewCredential] = useState("");
  const [newSharedPassword, setNewSharedPassword] = useState("");
  const [keyPassphrase, setKeyPassphrase] = useState("");
  const [keyPassphraseConfirmation, setKeyPassphraseConfirmation] = useState("");
  const [keyPassphraseOpen, setKeyPassphraseOpen] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [masterPassword, setMasterPassword] = useState("");
  const [masterConfirmation, setMasterConfirmation] = useState("");
  const [loading, setLoading] = useState(true);
  const [keyOptionsStatus, setKeyOptionsStatus] = useState<"loading" | "ready" | "failed">("loading");
  const [credentialOptionsStatus, setCredentialOptionsStatus] = useState<"loading" | "ready" | "locked" | "failed">("loading");
  const [busy, setBusy] = useState(false);
  const [vaultBusy, setVaultBusy] = useState(false);
  const [localError, setLocalError] = useState("");

  function clearKeyPassphrase() {
    setKeyPassphrase("");
    setKeyPassphraseConfirmation("");
  }

  function clearPasswordSecrets() {
    setPassword("");
    setNewSharedPassword("");
    setMasterPassword("");
    setMasterConfirmation("");
  }

  function clearSecrets() {
    clearPasswordSecrets();
    clearKeyPassphrase();
  }

  function applyCredentialState(status: PasswordVaultStatus, listed: Credential[]) {
    const passwordCredentials = listed.filter((credential) => credential.kind === "password");
    setKeyCredentials(listed.filter((credential) => credential.kind === "key_passphrase"));
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
    setKeyOptionsStatus("loading");
    setCredentialOptionsStatus("loading");
    setKeyState("loading");
    setPreferredSuperseded(false);

    let active = true;
    const applyKeys = (identities: KeyItem[], available: boolean) => {
      const preferred = preferredKey === null
        ? undefined
        : identities.find(
            (candidate) =>
              candidate.id === preferredKey.privateKeyId &&
              candidate.relativePath === preferredKey.privateRelativePath,
          );
      const direct = directIdentityFields(detail).filter((field) =>
        field.values.some(isConcreteIdentityValue),
      );
      setPrivateKeys(identities);
      let preferredAlreadyApplied = false;
      if (direct.length > 1) {
        setKeyState("complex");
        setSelectedKey("");
        setInitialKey("");
      } else if (direct.length === 1) {
        const configured = formatValues(direct[0]!.values);
        const matched = identities.find((candidate) => keyConfigValue(candidate) === configured);
        if (!available || matched === undefined) {
          setKeyState("custom");
          setCustomKey(configured);
          setSelectedKey("__custom__");
          setInitialKey("__custom__");
        } else {
          setKeyState("editable");
          setSelectedKey(preferred?.id ?? matched.id);
          setInitialKey(matched.id);
          preferredAlreadyApplied = preferred?.id === matched.id;
        }
      } else {
        setKeyState(available ? "editable" : "loading");
        setSelectedKey(available ? preferred?.id ?? "" : "");
        setInitialKey("");
      }
      if (preferredAlreadyApplied) onPreferredKeyApplied?.();
    };

    if (savedState !== undefined) {
      const keysReady = savedState.keys.status === "ready";
      const identities = savedState.keys.status === "ready" ? savedState.keys.value : [];
      applyKeys(identities, keysReady);
      setKeyOptionsStatus(keysReady ? "ready" : "failed");
      const status = savedState.vault.status === "ready" ? savedState.vault.value : null;
      setVault(status);
      setEligibility(savedState.eligibility.status === "ready" ? savedState.eligibility.value : null);
      if (status !== null && savedState.credentials.status === "ready") {
        applyCredentialState(status, savedState.credentials.value);
        setCredentialOptionsStatus("ready");
      } else {
        setCredentials([]);
        setKeyCredentials([]);
        setAssigned(status?.aliases.includes(identity.alias) ?? false);
        setAssignedCredential("");
        setCredentialOptionsStatus(savedState.credentials.status === "locked" ? "locked" : "failed");
      }
      setLoading(false);
      return () => {
        active = false;
        clearSecrets();
      };
    }

    void Promise.all([
      keys.inventory(),
      secrets.passwordVault(),
      secrets.passwordEligibility(identity.alias),
    ]).then(async ([inventory, status, nextEligibility]) => {
      const listed = status.unlocked ? (await secrets.credentials()).credentials : [];
      if (!active) return;

      applyKeys(selectablePrivateKeys(inventory), true);
      setKeyOptionsStatus("ready");
      setVault(status);
      setEligibility(nextEligibility);
      applyCredentialState(status, listed);
      setCredentialOptionsStatus(status.unlocked ? "ready" : "locked");
      setLoading(false);
    }).catch(() => {
      if (!active) return;
      clearSecrets();
      setLocalError(t("conn.basicOptionsFailed"));
      setKeyOptionsStatus("failed");
      setCredentialOptionsStatus("failed");
      setLoading(false);
    });
    return () => {
      active = false;
      clearSecrets();
    };
    // resetKey deliberately represents the server snapshot this draft belongs to.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey, keys, secrets, t, preferredKey, savedState]);

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
  const selectedPrivateKey = privateKeys.find((key) => key.id === selectedKey);
  const draftHasExplicitKey = keyState === "custom" || keyState === "complex" ||
    (keyState === "editable" && selectedKey !== "");
  const passwordCleanup = assigned && draftHasExplicitKey;
  const namedKeyPassphrase = selectedPrivateKey === undefined
    ? undefined
    : keyCredentials.find((credential) => credential.uses.includes(selectedPrivateKey.relativePath));
  const dedicatedKeyPassphrase = selectedPrivateKey !== undefined &&
    (vault?.dedicatedKeyPassphrases ?? []).includes(selectedPrivateKey.relativePath);
  const keyPassphraseStorageState = dedicatedKeyPassphrase
    ? "dedicated"
    : namedKeyPassphrase === undefined
      ? "none"
      : `named:${namedKeyPassphrase.name}`;
  const keyPassphraseDisclosureSubject = selectedPrivateKey?.encrypted === true
    ? selectedPrivateKey.id
    : "";
  const otherNamedKeyUses = namedKeyPassphrase === undefined || selectedPrivateKey === undefined
    ? []
    : namedKeyPassphrase.uses.filter((subject) => subject !== selectedPrivateKey.relativePath);

  const hostError = hostName.inherit
    ? ""
    : hostName.value === ""
      ? t("conn.createHostRequired")
      : validHostNameInput(hostName.value)
        ? ""
        : t("conn.createHostInvalid");
  const userError = user.value !== "" && /[\s\p{Cc}]/u.test(user.value) ? t("conn.createUserInvalid") : "";
  const parsedPort = Number(port.value);
  const portError = port.inherit || (/^\d+$/.test(port.value) && parsedPort >= 1 && parsedPort <= 65535)
    ? ""
    : t("conn.createPortInvalid");

  function effectivePassword(): UpdateConnectionPassword {
    if (passwordCleanup) return { kind: "remove" };
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
  const keyPassphraseChange: UpdateConnectionKeyPassphrase =
    selectedPrivateKey !== undefined && selectedPrivateKey.encrypted && keyPassphrase !== ""
      ? { kind: "set_dedicated", keyId: selectedPrivateKey.id, passphrase: keyPassphrase }
      : { kind: "unchanged" };
  const changesPassword = passwordChange.kind !== "unchanged";
  const hasKeyPassphraseDraft = keyPassphrase !== "" || keyPassphraseConfirmation !== "";
  const keyPassphraseMatches = keyPassphrase === keyPassphraseConfirmation;
  const keyPassphraseValid = !hasKeyPassphraseDraft || (keyPassphrase !== "" && keyPassphraseMatches);
  const nonIdentityBlockers = (eligibility?.blockers ?? []).filter((notice) => notice.code !== "identity_file_configured");
  const passwordAllowed = passwordChange.kind === "remove" || passwordChange.kind === "unchanged" ||
    (!draftHasExplicitKey && nonIdentityBlockers.length === 0);
  const dirty = hostNameChange !== undefined || userChange !== undefined || portChange !== undefined ||
    identityFileChange !== undefined || changesPassword || hasKeyPassphraseDraft;
  const passwordResourcesReady = vault?.unlocked === true && credentialOptionsStatus === "ready" && eligibility !== null;
  const keyPassphraseResourcesReady = vault?.unlocked === true && credentialOptionsStatus === "ready" && keyOptionsStatus === "ready";
  const vaultAllowsConfig = vault === null || vault.unlocked;
  const canSave = !disabled && !loading && !busy && vaultAllowsConfig && dirty &&
    hostError === "" && userError === "" && portError === "" && passwordAllowed &&
    keyPassphraseValid && (!changesPassword || passwordResourcesReady) &&
    (!hasKeyPassphraseDraft || keyPassphraseResourcesReady);

  useEffect(() => {
    if (credentialOptionsStatus !== "ready" || keyPassphraseDisclosureSubject === "") {
      return;
    }
    setKeyPassphraseOpen(keyPassphraseStorageState === "none");
  }, [credentialOptionsStatus, keyPassphraseDisclosureSubject, keyPassphraseStorageState]);

  useEffect(() => {
    if (!draftHasExplicitKey) return;
    clearPasswordSecrets();
    setPasswordAction("unchanged");
    setConfirmRemove(false);
    setNewCredential("");
  }, [draftHasExplicitKey]);

  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);

  const discardDraft = useCallback(() => {
    setHostName(initial.hostName);
    setUser(initial.user);
    setPort(initial.port);
    setSelectedKey(initialKey);
    setPasswordAction("unchanged");
    setConfirmRemove(false);
    setNewCredential("");
    setPassword("");
    setNewSharedPassword("");
    setMasterPassword("");
    setMasterConfirmation("");
    setKeyPassphrase("");
    setKeyPassphraseConfirmation("");
    setLocalError("");
  }, [initial, initialKey]);

  useEffect(() => {
    onDiscardReady?.(discardDraft);
    return () => onDiscardReady?.(null);
  }, [discardDraft, onDiscardReady]);

  function choosePasswordAction(action: PasswordAction) {
    clearPasswordSecrets();
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
      setCredentialOptionsStatus(status.unlocked ? "ready" : "locked");
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
      keyPassphrase: keyPassphraseChange,
    };
    if (hostNameChange !== undefined) request.hostName = hostNameChange;
    if (userChange !== undefined) request.user = userChange;
    if (portChange !== undefined) request.port = portChange;
    if (identityFileChange !== undefined) request.identityFile = identityFileChange;

    setBusy(true);
    setLocalError("");
    try {
      await onSave(request);
      if (keyPassphraseChange.kind !== "unchanged") {
        setKeyPassphraseOpen(false);
      }
      if (preferredKey !== null && (
        preferredSuperseded ||
        (identityFileChange?.action === "set" && identityFileChange.keyId === preferredKey.privateKeyId)
      )) {
        onPreferredKeyApplied?.();
      }
      clearSecrets();
      setPasswordAction("unchanged");
      setConfirmRemove(false);
      setNewCredential("");
      if (onRequestRefresh !== undefined) {
        await onRequestRefresh();
      } else if (savedState === undefined) {
        // A password-only transaction leaves ssh_config byte-for-byte unchanged,
        // so the detail snapshot key cannot reset this component for us. Refresh
        // the vault side explicitly and return the action control to unchanged.
        try {
          const status = await secrets.passwordVault();
          const listed = status.unlocked ? (await secrets.credentials()).credentials : [];
          setVault(status);
          applyCredentialState(status, listed);
          setCredentialOptionsStatus(status.unlocked ? "ready" : "locked");
        } catch {
          setCredentialOptionsStatus("failed");
          setLocalError(t("conn.basicRefreshFailed"));
        }
      }
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
  const passwordBlockers = nonIdentityBlockers;
  const passwordWarnings = (eligibility?.warnings ?? []).filter(
    (notice) => notice.code !== "identity_file_configured",
  );
  const serverHostError = problem?.code === "connection_hostname_invalid" ? t("conn.createHostInvalid") : "";
  const serverUserError = problem?.code === "connection_user_invalid" ? t("conn.createUserInvalid") : "";
  const serverPortError = problem?.code === "connection_port_invalid" ? t("conn.createPortInvalid") : "";
  const serverKeyError = problem?.code === "identity_file_invalid" ? t("conn.basicServerKeyInvalid") : "";
  const serverKeyPassphraseError = problem?.code === "wrong_passphrase"
    ? t("conn.basicKeyPassphraseWrong")
    : problem?.code === "external_change"
      ? t("conn.basicKeyPassphraseChanged")
      : "";
  const serverPasswordError = problem?.code === "credential_already_exists"
    ? t("conn.basicCredentialExists")
    : problem?.code === "unknown_credential"
      ? t("conn.basicCredentialMissing")
      : problem?.code === "password_missing"
        ? t("conn.basicPasswordMissing")
        : problem?.code === "password_ineligible"
          ? t("conn.basicPasswordBlocked")
          : problem?.code === "password_empty"
            ? t("conn.createNeedConnectionPassword")
            : "";

  return (
    <form className="flex flex-col gap-4" onSubmit={(event) => void submit(event)}>
      {localError === "" || problem !== null ? null : <Notice tone="danger">{localError}</Notice>}

      <fieldset disabled={disabled} className="contents">
      <section className="flex flex-col gap-2" aria-labelledby="basic-connection-heading">
        <h3 id="basic-connection-heading" className={sectionHeading}>{t("conn.basicConnection")}</h3>
        <Card>
          <Row
            label={t("conn.basicHostName")}
            hint={sourceText(hostName.state, t)}
            warning={hostName.state.origin === "complex" ? t("conn.basicComplex", { keyword: "HostName" }) : hostError || serverHostError || undefined}
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
            warning={user.state.origin === "complex" ? t("conn.basicComplex", { keyword: "User" }) : userError || serverUserError || undefined}
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
            warning={port.state.origin === "complex" ? t("conn.basicComplex", { keyword: "Port" }) : portError || serverPortError || undefined}
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
            warning={serverKeyError || undefined}
            hint={keyState === "custom"
              ? t("conn.basicCustomKey", { path: customKey })
              : keyState === "complex"
                ? t("conn.basicComplexKey")
                : draftHasExplicitKey
                  ? t("conn.basicThisConnection")
                  : t("conn.basicAgentOrInherited")}
          >
            <select
              aria-label={t("conn.basicPrivateKey")}
              value={selectedKey}
              disabled={loading || keyOptionsStatus !== "ready" || keyState === "custom" || keyState === "complex"}
              onChange={(event) => {
                const value = event.target.value;
                clearKeyPassphrase();
                setSelectedKey(value);
                const superseded = preferredKey !== null && value !== preferredKey.privateKeyId;
                setPreferredSuperseded(superseded);
                // 戻した先がサーバーの現在値なら保存操作は発生しない。その場で
                // handoff を破棄しても、effect の再初期化は同じ現在値を選ぶだけである。
                if (superseded && value === initialKey) onPreferredKeyApplied?.();
              }}
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
          {preferredKey !== null && identityFileChange?.action === "set" &&
          identityFileChange.keyId === preferredKey.privateKeyId ? (
            <p className="border-t border-hairline px-3 py-2 text-xs text-notice-ink">
              {t("conn.basicGeneratedKeyStaged", { path: preferredKey.privateRelativePath })}
            </p>
          ) : null}

          {vault?.unlocked === true && credentialOptionsStatus === "ready" &&
          keyState === "editable" && selectedPrivateKey !== undefined && selectedPrivateKey.encrypted ? (
            <details
              open={keyPassphraseOpen}
              onToggle={(event) => setKeyPassphraseOpen(event.currentTarget.open)}
              className="border-t border-hairline"
            >
              <summary className="cursor-pointer px-3 py-3 text-sm font-medium text-ink">
                {t("conn.basicManageKeyPassphrase")}
              </summary>
              <div className="flex flex-col gap-3 border-t border-hairline px-3 py-3">
                <div>
                  <p className="text-sm text-ink-muted">{t("conn.basicKeyPassphraseHeading")}</p>
                  <p className={hintText}>
                    {dedicatedKeyPassphrase
                        ? t("conn.basicKeyPassphraseDedicated")
                        : namedKeyPassphrase !== undefined
                          ? t("conn.basicKeyPassphraseShared", { name: namedKeyPassphrase.name })
                          : t("conn.basicKeyPassphraseNone")}
                  </p>
                  {namedKeyPassphrase !== undefined && otherNamedKeyUses.length > 0 ? (
                    <p className={hintText}>
                      {t("conn.basicKeyPassphraseSharedOthers", { count: otherNamedKeyUses.length })}
                    </p>
                  ) : null}
                  {namedKeyPassphrase !== undefined ? (
                    <p className={hintText}>{t("conn.basicKeyPassphraseDetach")}</p>
                  ) : null}
                </div>

                <div className="grid gap-3 sm:grid-cols-2">
                  <PasswordField
                    label={t("conn.basicNewKeyPassphrase")}
                    value={keyPassphrase}
                    onChange={(value) => {
                      setKeyPassphrase(value);
                      setLocalError("");
                    }}
                  />
                  <PasswordField
                    label={t("conn.basicConfirmKeyPassphrase")}
                    value={keyPassphraseConfirmation}
                    onChange={(value) => {
                      setKeyPassphraseConfirmation(value);
                      setLocalError("");
                    }}
                  />
                </div>
                {hasKeyPassphraseDraft && !keyPassphraseValid ? (
                  <Notice tone="danger">{t("conn.basicKeyPassphraseMismatch")}</Notice>
                ) : null}
                <p className={hintText}>{t("conn.basicKeyPassphraseStoredNote")}</p>
                {serverKeyPassphraseError === "" ? null : (
                  <Notice tone="danger">{serverKeyPassphraseError}</Notice>
                )}
              </div>
            </details>
          ) : null}

          {selectedPrivateKey !== undefined && !selectedPrivateKey.encrypted ? (
            <p className={`border-t border-hairline px-3 py-3 ${hintText}`}>
              {t("conn.basicKeyPassphraseUnencrypted")}
            </p>
          ) : null}

          {draftHasExplicitKey ? (
            passwordCleanup ? (
              <div className="border-t border-hairline px-3 py-3">
                <Notice>{t("conn.basicPasswordCleanup")}</Notice>
              </div>
            ) : null
          ) : <div className="border-t border-hairline px-3 py-3">
            <div className="flex flex-col gap-3">
              <div>
                <p className="text-sm text-ink-muted">{t("conn.basicStoredPassword")}</p>
                {loading || credentialOptionsStatus === "loading" ? <p className={hintText}>{t("conn.createLoadingOptions")}</p> : null}
                {credentialOptionsStatus === "failed" ? (
                  <p className={hintText}>{t("conn.basicCredentialOptionsFailed")}</p>
                ) : null}
                {vault?.unlocked === true && credentialOptionsStatus === "ready" ? (
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

              {vault?.unlocked === true && credentialOptionsStatus === "ready" ? (
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
                    <Notice key={`${blocker.code}-${index}`} tone="danger">{eligibilityText(t, blocker.code)}</Notice>
                  ))}
                  {passwordWarnings.map((warning, index) => (
                    <Notice key={`${warning.code}-${index}`}>{eligibilityText(t, warning.code)}</Notice>
                  ))}
                  {serverPasswordError === "" ? null : <Notice tone="danger">{serverPasswordError}</Notice>}
                </>
              ) : null}
            </div>
          </div>}
        </Card>
      </section>

      <div className="flex items-center gap-3 border-t border-line bg-canvas py-3">
        {!dirty ? <p className={`grow ${hintText}`}>{t("conn.basicNothingChanged")}</p> :
          (changesPassword && !passwordResourcesReady) || (hasKeyPassphraseDraft && !keyPassphraseResourcesReady) ?
            <p className={`grow ${hintText}`}>{t("conn.basicNeedVault")}</p> :
            !passwordAllowed ? <p className={`grow ${hintText}`}>{t("conn.basicPasswordBlocked")}</p> : <span className="grow" />}
        <Button type="button" disabled={!dirty || busy} onClick={discardDraft}>
          {t("conn.discardChanges")}
        </Button>
        <Button type="submit" kind="primary" disabled={!canSave}>
          {busy ? t("conn.basicSaving") : t("conn.basicSave")}
        </Button>
      </div>
      </fieldset>
    </form>
  );
}
