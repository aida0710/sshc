import { useCallback, useEffect, useMemo, useState, useId, type FormEvent } from "react";
import type { Problem } from "../api/client";
import type { HostDetail, UpdateConnectionRequest } from "../api/config";
import { connectionSecretsApi, type ConnectionSecretsApi } from "./secretsApi";
import { useTranslate } from "../i18n/context";
import { keysApi, type KeysApi } from "../keys/api";
import { control, sectionHeading } from "../ui/form";
import { Notice } from "../ui/surface";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import type { BasicFieldState } from "./basicFields";
import type { GeneratedPrivateKeyHandoff } from "../keys/workflow";
import type { ConnectionSavedState } from "./connectionSavedState";
import { identityKey } from "./connectionBrowser";
import { BasicPasswordSection } from "./BasicPasswordSection";
import { BasicPrivateKeyField } from "./BasicPrivateKeyField";
import { BasicTOTPSection } from "./BasicTOTPSection";
import { DraftSaveBar } from "./DraftSaveBar";
import {
  clearedKeyPassphrase,
  clearedPasswordChoice,
  clearedPasswordSecrets,
  clearedSecrets,
  deriveBasicForm,
  initialDraft,
  keySelectionOf,
  type BasicDraft,
  type DraftField,
  type PasswordAction,
} from "./basicFormDraft";
import { useConnectionSecrets } from "./useConnectionSecrets";

type ConnectionBasicFormProps = {
  detail: HostDetail;
  problem: Problem | null;
  onSave: (request: UpdateConnectionRequest) => Promise<void>;
  keys?: Pick<KeysApi, "inventory">;
  secrets?: ConnectionSecretsApi;
  preferredKey?: GeneratedPrivateKeyHandoff | null | undefined;
  onPreferredKeyApplied?: (() => void) | undefined;
  savedState?: ConnectionSavedState | undefined;
  onDirtyChange?: ((dirty: boolean) => void) | undefined;
  onDiscardReady?: ((discard: (() => void) | null) => void) | undefined;
  onRequestRefresh?: (() => Promise<void>) | undefined;
  disabled?: boolean | undefined;
};

function sourceText(field: BasicFieldState, t: ReturnType<typeof useTranslate>): string {
  if (field.origin === "direct") return t("conn.basicThisConnection");
  if (field.origin === "default") return t("conn.basicSSHDefault");
  if (field.origin === "complex") return t("conn.basicReadOnlyAdvanced");
  const path = field.source?.path ?? field.source?.absolute ?? "";
  return t("conn.basicInheritedFrom", { path, line: field.source?.line ?? 0 });
}

function ConnectionField({ field, label, error, inheritLabel, onChange, onInherit, numeric = false }: {
  field: DraftField;
  label: string;
  error: string;
  inheritLabel: string;
  onChange: (value: string) => void;
  onInherit: () => void;
  numeric?: boolean;
}) {
  const t = useTranslate();
  const id = useId();
  const hint = field.state.origin === "direct" ? "" : sourceText(field.state, t);
  const warning = field.state.origin === "complex" ? t("conn.basicComplex", { keyword: field.state.keyword }) : error;
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <label htmlFor={id} className="text-sm text-ink-muted">{label}</label>
      <input
        id={id}
        type={numeric ? "number" : "text"}
        {...(numeric ? { min: 1, max: 65535 } : {})}
        value={field.value}
        disabled={!field.state.editable || field.state.origin === "complex"}
        onChange={(event) => onChange(event.target.value)}
        aria-invalid={warning !== "" || undefined}
        aria-describedby={hint !== "" || warning !== "" ? `${id}-detail` : undefined}
        className={control}
      />
      {hint === "" && warning === "" ? null : <div id={`${id}-detail`} className="space-y-1 text-xs">
        {hint === "" ? null : <p className="text-ink-muted">{hint}</p>}
        {warning === "" ? null : <p className="text-notice-ink">{warning}</p>}
      </div>}
      {field.state.origin === "direct" ? <button type="button" aria-pressed={field.inherit} onClick={onInherit} className={`self-start rounded py-1 text-left text-xs underline-offset-4 hover:text-accent hover:underline ${field.inherit ? "font-medium text-notice-ink" : "text-ink-muted"}`}>
        {field.inherit ? t("conn.basicKeepDirect") : inheritLabel}
      </button> : null}
    </div>
  );
}

// The basic view of one connection: host, user and port, which key it uses,
// and what the vault keeps for it. The draft is one object; what it means
// against the vault is worked out by deriveBasicForm on every render.
export function ConnectionBasicForm({
  detail,
  problem,
  onSave,
  keys = keysApi,
  secrets: secretsApi = connectionSecretsApi,
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
  const resetKey = `${identityKey(identity)}\u0000${detail.file.contents}`;
  const secrets = useConnectionSecrets({ detail, resetKey, keys, secrets: secretsApi, savedState });
  const keySelection = useMemo(
    () => keySelectionOf(detail, secrets.privateKeys, secrets.keyOptionsStatus === "ready", preferredKey),
    [detail, secrets.privateKeys, secrets.keyOptionsStatus, preferredKey],
  );
  const [draft, setDraft] = useState<BasicDraft>(() => initialDraft(detail));
  const [preferredSuperseded, setPreferredSuperseded] = useState(false);
  const [keyPassphraseOpen, setKeyPassphraseOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [routeConfirmationOpen, setRouteConfirmationOpen] = useState(false);
  const derived = deriveBasicForm(detail, draft, keySelection, secrets, t);
  const { setError } = secrets;

  function edit(patch: Partial<BasicDraft>, clearError = false) {
    setDraft((current) => ({ ...current, ...patch }));
    if (clearError) setError("");
  }

  // A different host, file revision or handed-off key starts the form over.
  useEffect(() => {
    setDraft(initialDraft(detail));
    setRouteConfirmationOpen(false);
    setPreferredSuperseded(false);
    return () => { setDraft((current) => ({ ...current, ...clearedSecrets })); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey, keys, secretsApi, t, preferredKey, savedState]);

  // The key list arrives after the rest of the draft; the select follows it.
  useEffect(() => {
    setDraft((current) => ({ ...current, selectedKey: keySelection.initialSelected }));
    if (keySelection.preferredAlreadyApplied) onPreferredKeyApplied?.();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [keySelection]);

  const keyPassphraseDisclosureSubject = derived.selectedPrivateKey?.encrypted === true ? derived.selectedPrivateKey.id : "";
  useEffect(() => {
    if (secrets.credentialOptionsStatus !== "ready" || keyPassphraseDisclosureSubject === "") return;
    setKeyPassphraseOpen(derived.keyPassphraseStorageState === "none");
  }, [secrets.credentialOptionsStatus, keyPassphraseDisclosureSubject, derived.keyPassphraseStorageState]);

  // A key and a stored password cannot both apply; choosing a key drops the
  // password change the user had started.
  useEffect(() => {
    if (!derived.draftHasExplicitKey) return;
    setDraft((current) => ({ ...current, ...clearedPasswordSecrets, ...clearedPasswordChoice }));
  }, [derived.draftHasExplicitKey]);

  useEffect(() => {
    onDirtyChange?.(derived.dirty);
  }, [derived.dirty, onDirtyChange]);

  const discardDraft = useCallback(() => {
    setDraft(initialDraft(detail, keySelection.initialKey));
    setError("");
    setRouteConfirmationOpen(false);
  }, [detail, keySelection.initialKey, setError]);

  useEffect(() => {
    onDiscardReady?.(discardDraft);
    return () => onDiscardReady?.(null);
  }, [discardDraft, onDiscardReady]);

  function selectKey(value: string) {
    edit({ selectedKey: value, ...clearedKeyPassphrase });
    const superseded = preferredKey !== null && value !== preferredKey.privateKeyId;
    setPreferredSuperseded(superseded);
    if (superseded && value === keySelection.initialKey) onPreferredKeyApplied?.();
  }

  function choosePasswordAction(action: PasswordAction) {
    edit({ ...clearedPasswordSecrets, confirmRemove: false, passwordAction: action }, true);
  }

  async function openVault() {
    await secrets.openVault(draft.masterPassword);
    edit(clearedSecrets);
  }

  async function save() {
    if (!canSave) return;
    const request = derived.request();
    const { identityFileChange, keyPassphraseChange } = derived;
    setBusy(true);
    setError("");
    try {
      await onSave(request);
      if (keyPassphraseChange.kind !== "unchanged") setKeyPassphraseOpen(false);
      if (preferredKey !== null && (
        preferredSuperseded ||
        (identityFileChange?.action === "set" && identityFileChange.keyId === preferredKey.privateKeyId)
      )) {
        onPreferredKeyApplied?.();
      }
      edit({ ...clearedSecrets, ...clearedPasswordChoice, totpAction: "unchanged" });
      if (onRequestRefresh !== undefined) await onRequestRefresh();
      else if (savedState === undefined) await secrets.refreshCredentials();
    } catch {
      edit(clearedSecrets);
      setError(t("conn.basicSaveFailed"));
    } finally {
      setBusy(false);
    }
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    if (!canSave) return;
    if (derived.confirmPasswordRoute || derived.confirmTOTPRoute) {
      setRouteConfirmationOpen(true);
      return;
    }
    void save();
  }

  const canSave = !disabled && !secrets.loading && !busy && derived.vaultAllowsConfig && derived.dirty && derived.valid;
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

  const field = (name: "hostName" | "user" | "port") => ({
    field: draft[name],
    onChange: (value: string) => edit({ [name]: { ...draft[name], value, inherit: false } }, true),
    onInherit: () => edit({ [name]: { ...draft[name], inherit: !draft[name].inherit } }),
  });

  return (
    <form className="flex flex-col gap-4" onSubmit={submit}>
      {secrets.error === "" || problem !== null ? null : <Notice tone="danger">{secrets.error}</Notice>}

      <fieldset disabled={disabled} className="contents">
      <section className="flex flex-col gap-2" aria-labelledby="basic-connection-heading">
        <h3 id="basic-connection-heading" className={sectionHeading}>{t("conn.basicConnection")}</h3>
        <div className="grid gap-x-4 gap-y-3 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <ConnectionField {...field("hostName")} label={t("conn.basicHostName")} error={derived.hostError || serverHostError} inheritLabel={t("conn.basicUseInheritedHost")} />
          </div>
          <ConnectionField {...field("user")} label={t("conn.basicUser")} error={derived.userError || serverUserError} inheritLabel={t("conn.basicUseInheritedUser")} />
          <ConnectionField {...field("port")} label={t("conn.basicPort")} error={derived.portError || serverPortError} inheritLabel={t("conn.basicUseInheritedPort")} numeric />
        </div>
      </section>

      <section className="flex flex-col gap-3 border-t border-line pt-4" aria-labelledby="basic-auth-heading">
        <h3 id="basic-auth-heading" className={sectionHeading}>{t("conn.basicAuthentication")}</h3>
        <div>
          <BasicPrivateKeyField
            draft={draft}
            derived={derived}
            keySelection={keySelection}
            secrets={secrets}
            preferredKey={preferredKey}
            serverKeyError={serverKeyError}
            serverKeyPassphraseError={serverKeyPassphraseError}
            passphraseOpen={keyPassphraseOpen}
            onPassphraseOpenChange={setKeyPassphraseOpen}
            onSelectKey={selectKey}
            onEdit={(patch) => edit(patch, true)}
          />
          <BasicPasswordSection
            draft={draft}
            derived={derived}
            secrets={secrets}
            serverPasswordError={serverPasswordError}
            onEdit={(patch) => edit(patch)}
            onChooseAction={choosePasswordAction}
            onOpenVault={() => void openVault()}
          />
          <BasicTOTPSection
            draft={draft}
            derived={derived}
            secrets={secrets}
            onEdit={(patch) => edit(patch, "totpAction" in patch)}
          />
        </div>
      </section>

      {derived.dirty ? <DraftSaveBar
        note={derived.needsVault
          ? t("conn.basicNeedVault")
          : !derived.passwordAllowed ? t("conn.basicPasswordBlocked") : ""}
        saveLabel={t("conn.basicSave")}
        saving={busy}
        saveDisabled={!canSave}
        discardDisabled={busy}
        onDiscard={discardDraft}
      /> : null}
      </fieldset>
      {routeConfirmationOpen ? (
        <ConfirmDialog
          id="connection-route-confirmation-heading"
          heading={t("conn.basicRouteConfirmHeading")}
          body={<p className="text-sm text-ink-muted">{t("conn.basicRouteConfirmBody")}</p>}
          confirmLabel={t("conn.basicRouteConfirmSave")}
          cancelLabel={t("conn.basicRouteConfirmCancel")}
          confirmKind="primary"
          onConfirm={() => {
            setRouteConfirmationOpen(false);
            void save();
          }}
          onCancel={() => setRouteConfirmationOpen(false)}
        />
      ) : null}
    </form>
  );
}
