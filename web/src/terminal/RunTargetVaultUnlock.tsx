import type { ReactNode } from "react";
import type { RemoteEnginesApi } from "../api/remoteEngines";
import { useTranslate } from "../i18n/context";
import { PasswordField } from "../ui/PasswordField";
import { Button } from "../ui/surface";
import { useRemoteVaultUnlockForm } from "./useRemoteVaultUnlockForm";

// RunTargetVaultUnlock asks for the Vault password of the run target in the
// terminal area itself, so that nothing covers the rest of the page. The field
// is not focused by itself: the panel appears without being asked for, and
// taking the focus would pull it away from wherever the user is typing.
export function RunTargetVaultUnlock({
  alias,
  api,
  onUnlocked,
  onOpenInNewTab,
  children,
}: {
  alias: string;
  api: RemoteEnginesApi;
  onUnlocked: () => void;
  onOpenInNewTab: () => void;
  // children are the panel's other actions, shown under the form.
  children: ReactNode;
}) {
  const t = useTranslate();
  const form = useRemoteVaultUnlockForm({ alias, api, translate: t, onUnlocked });

  if (form.vaultMissing === true) {
    return (
      <div className="flex flex-col gap-3">
        <p className="break-words text-sm text-ink-muted">{t("runTarget.vaultMissingBody", { alias })}</p>
        <div className="flex flex-wrap gap-2">
          <Button kind="primary" onClick={onOpenInNewTab}>{t("runTarget.openInNewTab")}</Button>
          {children}
        </div>
      </div>
    );
  }
  return (
    <form onSubmit={(event) => void form.unlock(event)} className="flex flex-col gap-3">
      <PasswordField label={t("runTarget.unlockPassword", { alias })} value={form.password} onChange={form.setPassword} error={form.error} disabled={form.busy} />
      <div className="flex flex-wrap gap-2">
        <Button type="submit" kind="primary" busy={form.busy} disabled={form.password === ""}>{t("runTarget.unlock")}</Button>
        {children}
      </div>
    </form>
  );
}
