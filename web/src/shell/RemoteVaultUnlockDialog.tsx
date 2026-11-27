import { dialogHeading } from "../ui/form";
import { useId, type RefObject } from "react";
import type { RemoteEnginesApi } from "../api/remoteEngines";
import { useTranslate } from "../i18n/context";
import { useRemoteVaultUnlockForm } from "../terminal/useRemoteVaultUnlockForm";
import { ModalShell } from "../ui/ModalShell";
import { PasswordField } from "../ui/PasswordField";
import { hasOnScreenKeyboard } from "../ui/softKeyboard";
import { Button } from "../ui/surface";
import { ActionArea } from "../ui/ActionArea";

// RemoteVaultUnlockDialog asks for the Vault password of a run-target engine
// when the user chooses to unlock it from the engine information dialog.
export function RemoteVaultUnlockDialog({
  alias,
  api,
  onUnlocked,
  onOpenVault,
  onClose,
  returnFocusRef,
}: {
  alias: string;
  api: RemoteEnginesApi;
  onUnlocked: () => void;
  // onOpenVault は、その sshc エンジンの Vault の画面を開く（Vault が無いときに作る）。
  onOpenVault: () => void;
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
      panelClassName="w-full max-w-sm p-4"
    >
      {form.vaultMissing === true ? (
        <div className="flex flex-col gap-3">
          <h2 id={headingId} className={dialogHeading}>{t("runTarget.vaultMissingHeading", { alias })}</h2>
          <p className="text-sm text-ink-muted">{t("runTarget.vaultMissingBody", { alias })}</p>
          <ActionArea placement="inline">
            <Button onClick={onClose}>{t("runTarget.unlockCancel")}</Button>
            <Button kind="primary" onClick={onOpenVault}>{t("runTarget.openVault")}</Button>
          </ActionArea>
        </div>
      ) : (
        <form onSubmit={(event) => void form.unlock(event)} className="flex flex-col gap-3">
          <h2 id={headingId} className={dialogHeading}>{t("runTarget.unlockHeading", { alias })}</h2>
          <p className="text-sm text-ink-muted">{t("runTarget.unlockBody", { alias })}</p>
          <PasswordField label={t("runTarget.unlockPassword", { alias })} value={form.password} onChange={form.setPassword} error={form.error} disabled={busy} autoFocus={!hasOnScreenKeyboard()} />
          <ActionArea placement="inline">
            <Button disabled={busy} onClick={onClose}>{t("runTarget.unlockCancel")}</Button>
            <Button type="submit" kind="primary" busy={busy} busyLabel={t("busy.unlocking")} disabled={form.password === ""}>
              {t("runTarget.unlock")}
            </Button>
          </ActionArea>
        </form>
      )}
    </ModalShell>
  );
}
