import { useTranslate } from "../i18n/context";
import { ConnectionVaultUnlock } from "./ConnectionVaultUnlock";
import { CheckboxField, control } from "../ui/form";
import { FieldRow, FieldRowNote } from "../ui/FieldRow";
import { Icon } from "../ui/icons";
import { PasswordInput } from "../ui/PasswordField";
import { Notice } from "../ui/surface";
import { eligibilityText } from "./eligibilityText";
import type { BasicDraft, BasicFormDerived, PasswordAction } from "./basicFormDraft";
import type { ConnectionSecretsModel } from "./useConnectionSecrets";

// The choices that assign a password. While one is chosen, what stands in the way of using it is shown.
const assigningActions: readonly PasswordAction[] = ["dedicated_password", "saved_password", "new_shared_password"];

// The password the vault keeps for this host, as one select: what is assigned now (or nothing), and the
// ways to change it. Shown only while no explicit key is configured: a key and a stored password would
// contradict each other, so choosing a key removes the password instead.
export function BasicPasswordSection({
  draft, derived, secrets, serverPasswordError, onEdit, onChooseAction, onOpenVault,
}: {
  draft: BasicDraft;
  derived: BasicFormDerived;
  secrets: Pick<ConnectionSecretsModel, "vault" | "credentials" | "credentialOptionsStatus" | "eligibility" | "loading" | "vaultBusy" | "unlocked">;
  serverPasswordError: string;
  onEdit: (patch: Partial<BasicDraft>) => void;
  onChooseAction: (action: PasswordAction) => void;
  onOpenVault: () => void;
}) {
  const t = useTranslate();
  const { vault, credentials, credentialOptionsStatus } = secrets;
  if (derived.draftHasExplicitKey) {
    return derived.passwordCleanup ? (
      <FieldRowNote density="editor">
        <Notice>{t("conn.basicPasswordCleanup")}</Notice>
      </FieldRowNote>
    ) : null;
  }
  const label = t("conn.basicPassword");
  // Warnings such as an unknown host key only matter once a password is used, so nothing is said before.
  const passwordInUse = credentials.assigned || assigningActions.includes(draft.passwordAction);
  // A stale binding means the host, user or port changed after the password was assigned, so it is not
  // used until the Basic settings are saved again (which asks to confirm the current route).
  const stale = credentials.assigned && draft.passwordAction === "unchanged" && secrets.eligibility?.passwordBinding === "stale";
  const current = !credentials.assigned
    ? t("conn.basicNotAssigned")
    : credentials.assignedCredential === ""
      ? t("conn.basicPasswordDedicated")
      : t("conn.basicPasswordNamed", { name: credentials.assignedCredential });

  if (!secrets.unlocked) {
    return (
      <FieldRow density="editor" label={label} interactiveChildren>
        {vault !== null && !vault.unlocked ? (
          <ConnectionVaultUnlock
            vault={vault}
            masterPassword={draft.masterPassword}
            onMasterPasswordChange={(value) => onEdit({ masterPassword: value })}
            masterConfirmation={draft.masterConfirmation}
            onMasterConfirmationChange={(value) => onEdit({ masterConfirmation: value })}
            canOpen={derived.canOpenVault}
            busy={secrets.vaultBusy}
            onOpen={onOpenVault}
          />
        ) : (
          <p className="pt-1.5 text-[13px] text-ink-faint">
            {credentialOptionsStatus === "failed" ? t("conn.basicCredentialOptionsFailed") : t("conn.createLoadingOptions")}
          </p>
        )}
      </FieldRow>
    );
  }

  return (
    <>
      <FieldRow density="editor" label={label}>
        <select
          aria-label={label}
          value={draft.passwordAction}
          onChange={(event) => onChooseAction(event.target.value as PasswordAction)}
          className={control}
        >
          <option value="unchanged">{current}</option>
          <option value="saved_password">{t("conn.basicChooseSaved")}</option>
          <option value="dedicated_password">{t(credentials.assigned ? "conn.basicReplaceDedicated" : "conn.createDedicatedPassword")}</option>
          <option value="new_shared_password">{t("conn.createNewSharedPassword")}</option>
          {credentials.assigned ? <option value="remove">{t("conn.basicUnassign")}</option> : null}
        </select>
      </FieldRow>

      {draft.passwordAction === "dedicated_password" ? (
        <FieldRow density="editor" label={t("conn.createConnectionPassword")} hint={t("conn.basicEmptyPasswordUnchanged")} interactiveChildren>
          <PasswordInput
            label={t("conn.createConnectionPassword")}
            value={draft.password}
            onChange={(value) => onEdit({ password: value })}
          />
        </FieldRow>
      ) : draft.passwordAction === "saved_password" ? (
        <FieldRow density="editor" label={t("conn.createChooseSavedPassword")}>
          <select value={derived.savedCredential} onChange={(event) => onEdit({ savedCredential: event.target.value })} className={control}>
            {credentials.passwords.length === 0 ? <option value="">{t("conn.createNoSavedPasswords")}</option> : null}
            {credentials.passwords.map((credential) => <option key={credential.name} value={credential.name}>{credential.name}</option>)}
          </select>
        </FieldRow>
      ) : draft.passwordAction === "new_shared_password" ? (
        <>
          <FieldRow density="editor" label={t("conn.createSavedPasswordName")}>
            <input value={draft.newCredential} onChange={(event) => onEdit({ newCredential: event.target.value })} className={control} />
          </FieldRow>
          <FieldRow density="editor" label={t("conn.createNewPassword")} interactiveChildren>
            <PasswordInput label={t("conn.createNewPassword")} value={draft.newSharedPassword} onChange={(value) => onEdit({ newSharedPassword: value })} />
          </FieldRow>
        </>
      ) : draft.passwordAction === "remove" ? (
        <FieldRowNote density="editor">
          <CheckboxField label={t("conn.basicConfirmRemove")} checked={draft.confirmRemove} onChange={(checked) => onEdit({ confirmRemove: checked })} tone="danger" />
        </FieldRowNote>
      ) : null}

      {stale || (passwordInUse && (derived.passwordBlockers.length > 0 || derived.passwordWarnings.length > 0)) || serverPasswordError !== "" ? (
        <FieldRowNote density="editor">
          {stale ? <NoteLine>{t("conn.basicPasswordStale")}</NoteLine> : null}
          {passwordInUse ? derived.passwordBlockers.map((blocker, index) => (
            <NoteLine key={`${blocker.code}-${index}`} tone="danger">{eligibilityText(t, blocker.code)}</NoteLine>
          )) : null}
          {passwordInUse ? derived.passwordWarnings.map((warning, index) => (
            <NoteLine key={`${warning.code}-${index}`}>{eligibilityText(t, warning.code)}</NoteLine>
          )) : null}
          {serverPasswordError === "" ? null : <NoteLine tone="danger">{serverPasswordError}</NoteLine>}
        </FieldRowNote>
      ) : null}
    </>
  );
}

// NoteLine is one warning under a row: a marked, coloured line rather than a box.
function NoteLine({ tone = "notice", children }: { tone?: "notice" | "danger"; children: string }) {
  return (
    <p role={tone === "danger" ? "alert" : "status"} className={`flex gap-1.5 text-xs leading-[18px] ${tone === "danger" ? "text-danger" : "text-notice-ink"}`}>
      <Icon name="warning" className="mt-0.5 size-3.5 shrink-0" />
      <span>{children}</span>
    </p>
  );
}
