import { useCallback, useEffect, useMemo, useState, type FormEvent, type ReactNode } from "react";
import type { Problem } from "../api/client";
import type { HostDetail, UpdateConnectionRequest } from "../api/config";
import { connectionSecretsApi, type ConnectionSecretsApi } from "./secretsApi";
import { useTranslate } from "../i18n/context";
import { keysApi, type KeysApi } from "../keys/api";
import { Notice } from "../ui/surface";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import type { GeneratedPrivateKeyHandoff } from "../keys/workflow";
import type { ConnectionSavedState } from "./connectionSavedState";
import { draftResetKey } from "./draftResetKey";
import { BasicPasswordSection } from "./BasicPasswordSection";
import { BasicPrivateKeyField } from "./BasicPrivateKeyField";
import { BasicTOTPSection } from "./BasicTOTPSection";
import { ConnectionDestinationFields } from "./ConnectionDestinationFields";
import { EditorSection } from "./EditorSection";
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
import { useDraftSave } from "./useDraftSave";
import { useReportDirty } from "./useReportDirty";

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
  disabled?: boolean | undefined;
  // checks は、認証の節の最後に置く接続の確認（ConnectionChecks）。保存済みの設定を試すもので、下書きとは別。
  checks?: ReactNode;
};

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
  disabled = false,
  checks = null,
}: ConnectionBasicFormProps) {
  const t = useTranslate();
  const identity = detail.form.entry.identity;
  const resetKey = draftResetKey(identity, detail.file.contents);
  const secrets = useConnectionSecrets({ detail, resetKey, keys, secrets: secretsApi, savedState });
  const keySelection = useMemo(
    () => keySelectionOf(detail, secrets.privateKeys, secrets.keyOptionsStatus === "ready", preferredKey),
    [detail, secrets.privateKeys, secrets.keyOptionsStatus, preferredKey],
  );
  const [draft, setDraft] = useState<BasicDraft>(() => initialDraft(detail));
  const [preferredSuperseded, setPreferredSuperseded] = useState(false);
  const [keyPassphraseOpen, setKeyPassphraseOpen] = useState(false);
  const [routeConfirmationOpen, setRouteConfirmationOpen] = useState(false);
  const derived = deriveBasicForm(detail, draft, keySelection, secrets, t);
  const { setError } = secrets;
  // The part of the draft that stays after a save: where to connect and which
  // key. A save wipes the secrets and resets the password and TOTP choices, so
  // they are left out; wiping them would change the reference, and the written
  // draft would count as unsaved again while the page reads it back.
  const savedDraft = useMemo(
    () => ({ hostName: draft.hostName, user: draft.user, port: draft.port, selectedKey: draft.selectedKey }),
    [draft.hostName, draft.user, draft.port, draft.selectedKey],
  );
  const { saving, written, save: saveDraft } = useDraftSave({ draft: savedDraft, saved: detail });
  const dirty = !written && derived.dirty;

  function edit(patch: Partial<BasicDraft>, clearError = false) {
    setDraft((current) => ({ ...current, ...patch }));
    if (clearError) setError("");
  }

  // A different host or file revision (resetKey) or handed-off key starts the
  // form over. So does anything that makes useConnectionSecrets read the keys and
  // the vault again (keys, secretsApi, savedState, which a save refreshes), so
  // the draft starts from what was just read. `detail` is read through
  // resetKey: refetching the same revision keeps the draft. The display
  // language is not a reason to throw the draft away.
  useEffect(() => {
    setDraft(initialDraft(detail));
    setRouteConfirmationOpen(false);
    setPreferredSuperseded(false);
    return () => { setDraft((current) => ({ ...current, ...clearedSecrets })); };
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey, keys, secretsApi, preferredKey, savedState]);

  // The key list arrives after the rest of the draft; the select follows it.
  // onPreferredKeyApplied only tells the parent; a new callback from a parent
  // render must not pick the key again over the user's choice.
  useEffect(() => {
    setDraft((current) => ({ ...current, selectedKey: keySelection.initialSelected }));
    if (keySelection.preferredAlreadyApplied) onPreferredKeyApplied?.();
    // oxlint-disable-next-line react-hooks/exhaustive-deps
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

  useReportDirty(dirty, onDirtyChange);

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
    setError("");
    if (!await saveDraft(() => onSave(request))) {
      edit(clearedSecrets);
      setError(t("conn.basicSaveFailed"));
      return;
    }
    if (keyPassphraseChange.kind !== "unchanged") setKeyPassphraseOpen(false);
    if (preferredKey !== null && (
      preferredSuperseded ||
      (identityFileChange?.action === "set" && identityFileChange.keyId === preferredKey.privateKeyId)
    )) {
      onPreferredKeyApplied?.();
    }
    edit({ ...clearedSecrets, ...clearedPasswordChoice, totpAction: "unchanged" });
    // The page reads the saved connection again, savedState included. Without
    // one, the form reads what the vault now holds for this host itself.
    if (savedState === undefined) await secrets.refreshCredentials();
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

  const canSave = !disabled && !secrets.loading && !saving && derived.vaultAllowsConfig && dirty && derived.valid;
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

  const destination = (name: "hostName" | "user" | "port", error: string) => ({
    draft: draft[name],
    error,
    onChange: (next: DraftField) => edit({ [name]: next }, true),
  });
  const file = detail.form.entry.file;

  return (
    <form className="flex flex-col" onSubmit={submit}>
      {secrets.error === "" || problem !== null ? null : <Notice tone="danger">{secrets.error}</Notice>}

      <fieldset disabled={disabled} className="contents">
      <EditorSection title={t("conn.basicConnection")} meta={<span title={file.absolute}>{file.path ?? file.absolute}</span>}>
        <ConnectionDestinationFields
          hostName={destination("hostName", derived.hostError || serverHostError)}
          user={destination("user", derived.userError || serverUserError)}
          port={destination("port", derived.portError || serverPortError)}
        />
      </EditorSection>

      <EditorSection title={t("conn.basicAuthentication")}>
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
        {checks}
      </EditorSection>

      {dirty ? <DraftSaveBar
        note={derived.needsVault
          ? t("conn.basicNeedVault")
          : !derived.passwordAllowed ? t("conn.basicPasswordBlocked") : ""}
        saveLabel={t("conn.basicSave")}
        saving={saving}
        saveDisabled={!canSave}
        discardDisabled={saving}
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
