import { useTranslate } from "../i18n/context";
import { CheckboxField, control } from "../ui/form";
import { FieldRow, FieldRowNote } from "../ui/FieldRow";
import { PasswordField, PasswordInput } from "../ui/PasswordField";
import { Button, Notice } from "../ui/surface";
import { eligibilityText } from "./eligibilityText";
import type { BasicDraft, BasicFormDerived, PasswordAction } from "./basicFormDraft";
import type { ConnectionSecretsModel } from "./useConnectionSecrets";

// The password the vault keeps for this host. Shown only while no explicit
// key is configured: a key and a stored password would contradict each
// other, so choosing a key removes the password instead.
export function BasicPasswordSection({
  draft, derived, secrets, serverPasswordError, onEdit, onChooseAction, onOpenVault,
}: {
  draft: BasicDraft;
  derived: BasicFormDerived;
  secrets: Pick<ConnectionSecretsModel, "vault" | "credentials" | "credentialOptionsStatus" | "loading" | "vaultBusy" | "unlocked">;
  serverPasswordError: string;
  onEdit: (patch: Partial<BasicDraft>) => void;
  onChooseAction: (action: PasswordAction) => void;
  onOpenVault: () => void;
}) {
  const t = useTranslate();
  const { vault, credentials, credentialOptionsStatus } = secrets;
  if (derived.draftHasExplicitKey) {
    return derived.passwordCleanup ? (
      <div className="border-t border-line py-3.5">
        <Notice>{t("conn.basicPasswordCleanup")}</Notice>
      </div>
    ) : null;
  }
  return (
    <div className="border-t border-line">
      <FieldRow label={t("conn.basicStoredPassword")} interactiveChildren>
        <div className="flex flex-col gap-1 pt-1.5 text-[13px] text-ink-muted">
          {secrets.loading || credentialOptionsStatus === "loading" ? <p>{t("conn.createLoadingOptions")}</p> : null}
          {credentialOptionsStatus === "failed" ? <p>{t("conn.basicCredentialOptionsFailed")}</p> : null}
          {secrets.unlocked ? (
            <p>
              {credentials.assigned
                ? credentials.assignedCredential === ""
                  ? t("conn.basicAssignedDedicated")
                  : t("conn.basicAssignedNamed", { name: credentials.assignedCredential })
                : t("conn.basicNoPassword")}
            </p>
          ) : null}
        </div>
      </FieldRow>

      {/* Notices take the whole width: the controls' column is narrow beside the connection list. */}
      {vault !== null && !vault.unlocked ? (
        <div className="pb-3.5">
          <div className="flex flex-col gap-3 rounded-[9px] bg-notice p-3">
            <p className="text-sm text-notice-ink">
              {t(vault.exists ? "conn.basicVaultLocked" : "conn.basicVaultMissing")}
            </p>
            <PasswordField label={t("conn.createMasterPassword")} value={draft.masterPassword} onChange={(value) => onEdit({ masterPassword: value })} />
            {vault.exists ? null : (
              <PasswordField
                label={t("conn.createConfirmMaster")}
                value={draft.masterConfirmation}
                onChange={(value) => onEdit({ masterConfirmation: value })}
              />
            )}
            <Button kind="primary" className="self-start" disabled={!derived.canOpenVault} busy={secrets.vaultBusy} onClick={onOpenVault}>
              {t(vault.exists ? "conn.createUnlockVault" : "conn.createInitialiseVault")}
            </Button>
          </div>
        </div>
      ) : null}

      {secrets.unlocked ? (
        <>
          <FieldRow label={t("conn.basicPasswordAction")}>
            <select
              aria-label={t("conn.basicPasswordAction")}
              value={draft.passwordAction}
              onChange={(event) => onChooseAction(event.target.value as PasswordAction)}
              className={control}
            >
              <option value="unchanged">{t("conn.basicPasswordUnchanged")}</option>
              <option value="dedicated_password">{t(credentials.assigned ? "conn.basicReplaceDedicated" : "conn.createDedicatedPassword")}</option>
              <option value="saved_password">{t("conn.createSavedPassword")}</option>
              <option value="new_shared_password">{t("conn.createNewSharedPassword")}</option>
              {credentials.assigned ? <option value="remove">{t("conn.basicRemovePassword")}</option> : null}
            </select>
          </FieldRow>

          {draft.passwordAction === "dedicated_password" ? (
            <FieldRow label={t("conn.createConnectionPassword")} hint={t("conn.basicEmptyPasswordUnchanged")} interactiveChildren>
              <PasswordInput
                label={t("conn.createConnectionPassword")}
                value={draft.password}
                onChange={(value) => onEdit({ password: value })}
              />
            </FieldRow>
          ) : draft.passwordAction === "saved_password" ? (
            <FieldRow label={t("conn.createChooseSavedPassword")}>
              <select value={derived.savedCredential} onChange={(event) => onEdit({ savedCredential: event.target.value })} className={control}>
                {credentials.passwords.length === 0 ? <option value="">{t("conn.createNoSavedPasswords")}</option> : null}
                {credentials.passwords.map((credential) => <option key={credential.name} value={credential.name}>{credential.name}</option>)}
              </select>
            </FieldRow>
          ) : draft.passwordAction === "new_shared_password" ? (
            <>
              <FieldRow label={t("conn.createSavedPasswordName")}>
                <input value={draft.newCredential} onChange={(event) => onEdit({ newCredential: event.target.value })} className={control} />
              </FieldRow>
              <FieldRow label={t("conn.createNewPassword")} interactiveChildren>
                <PasswordInput label={t("conn.createNewPassword")} value={draft.newSharedPassword} onChange={(value) => onEdit({ newSharedPassword: value })} />
              </FieldRow>
            </>
          ) : null}

          {draft.passwordAction === "remove" ? (
            <FieldRowNote>
              <CheckboxField label={t("conn.basicConfirmRemove")} checked={draft.confirmRemove} onChange={(checked) => onEdit({ confirmRemove: checked })} tone="danger" />
            </FieldRowNote>
          ) : null}
          {derived.passwordBlockers.length > 0 || derived.passwordWarnings.length > 0 || serverPasswordError !== "" ? (
            <div className="flex flex-col gap-2 pb-3.5">
              {derived.passwordBlockers.map((blocker, index) => (
                <Notice key={`${blocker.code}-${index}`} tone="danger">{eligibilityText(t, blocker.code)}</Notice>
              ))}
              {derived.passwordWarnings.map((warning, index) => (
                <Notice key={`${warning.code}-${index}`}>{eligibilityText(t, warning.code)}</Notice>
              ))}
              {serverPasswordError === "" ? null : <Notice tone="danger">{serverPasswordError}</Notice>}
            </div>
          ) : null}
        </>
      ) : null}
    </div>
  );
}
