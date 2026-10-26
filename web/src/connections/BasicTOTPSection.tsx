import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";
import { FieldRow } from "../ui/FieldRow";
import type { BasicDraft, BasicFormDerived, TOTPAction } from "./basicFormDraft";
import type { ConnectionSecretsModel } from "./useConnectionSecrets";

// The one-time-password secret the vault answers with for this host.
export function BasicTOTPSection({ draft, derived, secrets, onEdit }: {
  draft: BasicDraft;
  derived: BasicFormDerived;
  secrets: Pick<ConnectionSecretsModel, "credentials" | "credentialOptionsStatus" | "loading" | "unlocked">;
  onEdit: (patch: Partial<Pick<BasicDraft, "totpAction" | "savedTOTP">>) => void;
}) {
  const t = useTranslate();
  const { credentials, credentialOptionsStatus } = secrets;
  // The section draws the line above its first row, which as a first child draws none; pt-3 gives that row
  // the same room as the others.
  return (
    <div className="border-t border-line pt-3">
      <FieldRow label={t("conn.basicStoredTOTP")} interactiveChildren>
        <div className="flex flex-col gap-1 pt-1.5 text-[13px] text-ink-muted">
          {secrets.loading || credentialOptionsStatus === "loading" ? <p>{t("conn.createLoadingOptions")}</p> : null}
          {credentialOptionsStatus === "failed" ? <p>{t("conn.basicCredentialOptionsFailed")}</p> : null}
          {secrets.unlocked ? (
            <p>
              {credentials.assignedTOTP === ""
                ? t("conn.basicNoTOTP")
                : t("conn.basicAssignedTOTP", { name: credentials.assignedTOTP })}
            </p>
          ) : null}
        </div>
      </FieldRow>

      {secrets.unlocked ? (
        <>
          <FieldRow label={t("conn.basicTOTPAction")} hint={draft.totpAction === "saved_totp" ? undefined : t("conn.basicTOTPNote")}>
            <select
              aria-label={t("conn.basicTOTPAction")}
              value={draft.totpAction}
              onChange={(event) => onEdit({ totpAction: event.target.value as TOTPAction })}
              className={control}
            >
              <option value="unchanged">{t("conn.basicTOTPUnchanged")}</option>
              <option value="saved_totp">{t("conn.basicUseSavedTOTP")}</option>
              {credentials.assignedTOTP === "" ? null : (
                <option value="remove">{t("conn.basicRemoveTOTP")}</option>
              )}
            </select>
          </FieldRow>

          {draft.totpAction === "saved_totp" ? (
            <FieldRow label={t("conn.basicChooseSavedTOTP")} hint={t("conn.basicTOTPNote")}>
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
      ) : null}
    </div>
  );
}
