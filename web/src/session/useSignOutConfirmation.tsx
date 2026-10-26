import { useState } from "react";
import { useLanguage } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { signOut } from "./signOut";

// useSignOutConfirmation asks for confirmation, signs this browser out and reloads.
// Leaving is a reload with nothing to recover from: the app then shows how to
// enter again from the terminal. The caller draws what starts it, and places the
// returned dialog and error.
export function useSignOutConfirmation() {
  const { t } = useLanguage();
  const [confirming, setConfirming] = useState(false);
  const signingOut = useAsyncOperation();

  async function leave() {
    setConfirming(false);
    const done = await signingOut.run(signOut, { describe: () => t("menu.signOutFailed") });
    if (done) window.location.reload();
  }

  const confirmation = confirming ? (
    <ConfirmDialog
      id="sign-out-heading"
      heading={t("menu.signOutConfirmHeading")}
      body={<p className="text-sm text-ink-muted">{t("menu.signOutConfirmBody")}</p>}
      confirmLabel={t("menu.signOut")}
      cancelLabel={t("menu.signOutCancel")}
      onCancel={() => setConfirming(false)}
      onConfirm={() => void leave()}
    />
  ) : null;

  return { requestSignOut: () => setConfirming(true), busy: signingOut.busy, error: signingOut.error, confirmation };
}
