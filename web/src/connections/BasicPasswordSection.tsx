import { useTranslate } from "../i18n/context";
import { CheckboxField, control, hintText } from "../ui/form";
import { PasswordField } from "../ui/PasswordField";
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
      <div className="border-t border-hairline py-3">
        <Notice>{t("conn.basicPasswordCleanup")}</Notice>
      </div>
    ) : null;
  }
  return (
    <div className="border-t border-hairline py-3">
      <div className="flex flex-col gap-3">
        <div>
          <p className="text-sm text-ink-muted">{t("conn.basicStoredPassword")}</p>
          {secrets.loading || credentialOptionsStatus === "loading" ? <p className={hintText}>{t("conn.createLoadingOptions")}</p> : null}
          {credentialOptionsStatus === "failed" ? (
            <p className={hintText}>{t("conn.basicCredentialOptionsFailed")}</p>
          ) : null}
          {secrets.unlocked ? (
            <p className={hintText}>
              {credentials.assigned
                ? credentials.assignedCredential === ""
                  ? t("conn.basicAssignedDedicated")
                  : t("conn.basicAssignedNamed", { name: credentials.assignedCredential })
                : t("conn.basicNoPassword")}
            </p>
          ) : null}
        </div>

        {vault !== null && !vault.unlocked ? (
          <div className="flex flex-col gap-3 rounded-lg border border-notice-line bg-notice p-3">
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
            <Button kind="primary" disabled={secrets.vaultBusy || !derived.canOpenVault} onClick={onOpenVault}>
              {t(vault.exists ? "conn.createUnlockVault" : "conn.createInitialiseVault")}
            </Button>
          </div>
        ) : null}

        {secrets.unlocked ? (
          <>
            <label className="flex flex-col gap-1">
              <span className="text-xs font-medium tracking-wide text-ink-muted">{t("conn.basicPasswordAction")}</span>
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
            </label>

            {draft.passwordAction === "dedicated_password" ? (
              <PasswordField
                label={t("conn.createConnectionPassword")}
                value={draft.password}
                onChange={(value) => onEdit({ password: value })}
                hint={t("conn.basicEmptyPasswordUnchanged")}
              />
            ) : draft.passwordAction === "saved_password" ? (
              <label className="flex flex-col gap-1">
                <span className="text-xs font-medium tracking-wide text-ink-muted">{t("conn.createChooseSavedPassword")}</span>
                <select value={derived.savedCredential} onChange={(event) => onEdit({ savedCredential: event.target.value })} className={control}>
                  {credentials.passwords.length === 0 ? <option value="">{t("conn.createNoSavedPasswords")}</option> : null}
                  {credentials.passwords.map((credential) => <option key={credential.name} value={credential.name}>{credential.name}</option>)}
                </select>
              </label>
            ) : draft.passwordAction === "new_shared_password" ? (
              <div className="grid gap-3 sm:grid-cols-2">
                <label className="flex flex-col gap-1">
                  <span className="text-xs font-medium tracking-wide text-ink-muted">{t("conn.createSavedPasswordName")}</span>
                  <input value={draft.newCredential} onChange={(event) => onEdit({ newCredential: event.target.value })} className={control} />
                </label>
                <PasswordField label={t("conn.createNewPassword")} value={draft.newSharedPassword} onChange={(value) => onEdit({ newSharedPassword: value })} />
              </div>
            ) : draft.passwordAction === "remove" ? (
              <CheckboxField label={t("conn.basicConfirmRemove")} checked={draft.confirmRemove} onChange={(checked) => onEdit({ confirmRemove: checked })} tone="danger" />
            ) : null}

            {derived.passwordBlockers.map((blocker, index) => (
              <Notice key={`${blocker.code}-${index}`} tone="danger">{eligibilityText(t, blocker.code)}</Notice>
            ))}
            {derived.passwordWarnings.map((warning, index) => (
              <Notice key={`${warning.code}-${index}`}>{eligibilityText(t, warning.code)}</Notice>
            ))}
            {serverPasswordError === "" ? null : <Notice tone="danger">{serverPasswordError}</Notice>}
          </>
        ) : null}
      </div>
    </div>
  );
}
