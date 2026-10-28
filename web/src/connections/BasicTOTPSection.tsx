import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";
import { FieldRow } from "../ui/FieldRow";
import type { BasicDraft, BasicFormDerived, TOTPAction } from "./basicFormDraft";
import type { ConnectionSecretsModel } from "./useConnectionSecrets";

// The one-time-password secret the vault answers with for this host, as one select: what is assigned now
// (or nothing), and the ways to change it.
export function BasicTOTPSection({ draft, derived, secrets, onEdit }: {
  draft: BasicDraft;
  derived: BasicFormDerived;
  secrets: Pick<ConnectionSecretsModel, "credentials" | "credentialOptionsStatus" | "loading" | "unlocked">;
  onEdit: (patch: Partial<Pick<BasicDraft, "totpAction" | "savedTOTP">>) => void;
}) {
  const t = useTranslate();
  const { credentials, credentialOptionsStatus } = secrets;
  const label = t("conn.basicTOTP");
  if (!secrets.unlocked) {
    // The vault is unlocked from the password row above.
    return (
      <FieldRow density="editor" label={label} interactiveChildren>
        <p className="pt-1.5 text-[13px] text-ink-faint">
          {credentialOptionsStatus === "failed"
            ? t("conn.basicCredentialOptionsFailed")
            : secrets.loading || credentialOptionsStatus === "loading"
              ? t("conn.createLoadingOptions")
              : t("conn.basicTOTPNeedsVault")}
        </p>
      </FieldRow>
    );
  }
  return (
    <>
      <FieldRow density="editor" label={label}>
        <select
          aria-label={label}
          value={draft.totpAction}
          onChange={(event) => onEdit({ totpAction: event.target.value as TOTPAction })}
          className={control}
        >
          <option value="unchanged">{credentials.assignedTOTP === "" ? t("conn.basicNotAssigned") : credentials.assignedTOTP}</option>
          <option value="saved_totp">{t("conn.basicChooseSaved")}</option>
          {credentials.assignedTOTP === "" ? null : <option value="remove">{t("conn.basicUnassign")}</option>}
        </select>
      </FieldRow>

      {draft.totpAction === "saved_totp" ? (
        <FieldRow density="editor" label={t("conn.basicChooseSavedTOTP")} hint={t("conn.basicTOTPNote")}>
          <select
            value={derived.savedTOTP}
            onChange={(event) => onEdit({ savedTOTP: event.target.value })}
            className={control}
          >
            {credentials.totps.length === 0 ? (
              <option value="">{t("conn.basicNoSavedTOTPs")}</option>
            ) : null}
            {credentials.totps.map((credential) => (
              <option key={credential.name} value={credential.name}>
                {credential.name}
              </option>
            ))}
          </select>
        </FieldRow>
      ) : null}
    </>
  );
}
