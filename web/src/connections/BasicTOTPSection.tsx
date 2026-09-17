import { useTranslate } from "../i18n/context";
import { control, hintText } from "../ui/form";
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
  return (
    <div className="border-t border-hairline py-3">
      <div className="flex flex-col gap-3">
        <div>
          <p className="text-sm text-ink-muted">{t("conn.basicStoredTOTP")}</p>
          {secrets.loading || credentialOptionsStatus === "loading" ? (
            <p className={hintText}>{t("conn.createLoadingOptions")}</p>
          ) : null}
          {credentialOptionsStatus === "failed" ? (
            <p className={hintText}>{t("conn.basicCredentialOptionsFailed")}</p>
          ) : null}
          {secrets.unlocked ? (
            <p className={hintText}>
              {credentials.assignedTOTP === ""
                ? t("conn.basicNoTOTP")
                : t("conn.basicAssignedTOTP", { name: credentials.assignedTOTP })}
            </p>
          ) : null}
        </div>

        {secrets.unlocked ? (
          <>
            <label className="flex flex-col gap-1">
              <span className="text-xs font-medium tracking-wide text-ink-muted">
                {t("conn.basicTOTPAction")}
              </span>
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
            </label>

            {draft.totpAction === "saved_totp" ? (
              <label className="flex flex-col gap-1">
                <span className="text-xs font-medium tracking-wide text-ink-muted">
                  {t("conn.basicChooseSavedTOTP")}
                </span>
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
              </label>
            ) : null}
            <p className={hintText}>{t("conn.basicTOTPNote")}</p>
          </>
        ) : null}
      </div>
    </div>
  );
}
