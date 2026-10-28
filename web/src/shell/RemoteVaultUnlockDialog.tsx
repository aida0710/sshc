import { dialogHeading } from "../ui/form";
import { useId, type RefObject } from "react";
import type { RemoteEnginesApi } from "../api/remoteEngines";
import { useTranslate } from "../i18n/context";
import { useRemoteVaultUnlockForm } from "../terminal/useRemoteVaultUnlockForm";
import { ModalShell } from "../ui/ModalShell";
import { PasswordField } from "../ui/PasswordField";
import { Button } from "../ui/surface";

// RemoteVaultUnlockDialog asks for the Vault password of a run-target engine
// when the user chooses to unlock it from the engine information dialog.
export function RemoteVaultUnlockDialog({
  alias,
  api,
  onUnlocked,
  onOpenInNewTab,
  onClose,
  returnFocusRef,
}: {
  alias: string;
  api: RemoteEnginesApi;
  onUnlocked: () => void;
  onOpenInNewTab: () => void;
  onClose: () => void;
  returnFocusRef?: RefObject<HTMLElement | null>;
}) {
  const t = useTranslate();
  const headingId = useId();
  const form = useRemoteVaultUnlockForm({ alias, api, translate: t, onUnlocked });
  const { busy } = form;

  return (
    <ModalShell
      labelledBy={headingId}
      onDismiss={onClose}
      dismissible={!busy}
      {...(returnFocusRef === undefined ? {} : { returnFocusRef })}
      panelClassName="w-full max-w-sm rounded-lg p-4"
    >
      {form.vaultMissing === true ? (
        <div className="flex flex-col gap-3">
          <h2 id={headingId} className={dialogHeading}>{t("runTarget.vaultMissingHeading", { alias })}</h2>
          <p className="text-sm text-ink-muted">{t("runTarget.vaultMissingBody", { alias })}</p>
          <div className="flex justify-end gap-2">
            <Button onClick={onClose}>{t("runTarget.unlockCancel")}</Button>
            <Button kind="primary" onClick={onOpenInNewTab}>{t("runTarget.openInNewTab")}</Button>
          </div>
        </div>
      ) : (
        <form onSubmit={(event) => void form.unlock(event)} className="flex flex-col gap-3">
          <h2 id={headingId} className={dialogHeading}>{t("runTarget.unlockHeading", { alias })}</h2>
          <p className="text-sm text-ink-muted">{t("runTarget.unlockBody", { alias })}</p>
          <PasswordField label={t("runTarget.unlockPassword", { alias })} value={form.password} onChange={form.setPassword} error={form.error} disabled={busy} autoFocus />
          <div className="flex justify-end gap-2">
            <Button disabled={busy} onClick={onClose}>{t("runTarget.unlockCancel")}</Button>
            <Button type="submit" kind="primary" disabled={busy || form.password === ""}>
              {busy ? t("runTarget.unlocking") : t("runTarget.unlock")}
            </Button>
          </div>
        </form>
      )}
    </ModalShell>
  );
}
