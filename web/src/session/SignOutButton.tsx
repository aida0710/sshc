import { useState } from "react";
import { useLanguage } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { Button, Notice } from "../ui/surface";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { signOut } from "./signOut";

// SignOutButton asks for confirmation, signs this browser out and reloads.
// Leaving is a reload with nothing to recover from: the app then shows how to
// enter again from the terminal. The menu and the lock screen both offer it,
// so a browser can be signed out without knowing the master password.
export function SignOutButton() {
  const { t } = useLanguage();
  const [confirming, setConfirming] = useState(false);
  const signingOut = useAsyncOperation();

  async function leave() {
    setConfirming(false);
    const done = await signingOut.run(signOut, { describe: () => t("menu.signOutFailed") });
    if (done) window.location.reload();
  }

  return (
    <>
      <div className="flex flex-wrap items-center gap-3">
        <Button kind="danger" disabled={signingOut.busy} onClick={() => setConfirming(true)}>{t("menu.signOut")}</Button>
        {signingOut.error === "" ? null : <Notice tone="danger">{signingOut.error}</Notice>}
      </div>
      {confirming ? (
        <ConfirmDialog
          id="sign-out-heading"
          heading={t("menu.signOutConfirmHeading")}
          body={<p className="text-sm text-ink-muted">{t("menu.signOutConfirmBody")}</p>}
          confirmLabel={t("menu.signOut")}
          cancelLabel={t("menu.signOutCancel")}
          onCancel={() => setConfirming(false)}
          onConfirm={() => void leave()}
        />
      ) : null}
    </>
  );
}
