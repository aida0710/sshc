import { useLanguage } from "../i18n/context";
import { Button, Notice } from "../ui/surface";
import { useSignOutConfirmation } from "./useSignOutConfirmation";

// SignOutButton is the lock screen's way out, so a browser can be signed out
// without knowing the master password. The Menu draws its own row for the same.
export function SignOutButton() {
  const { t } = useLanguage();
  const { requestSignOut, busy, error, confirmation } = useSignOutConfirmation();
  return (
    <>
      <div className="flex flex-wrap items-center gap-3">
        <Button kind="danger" disabled={busy} onClick={requestSignOut}>{t("menu.signOut")}</Button>
        {error === "" ? null : <Notice tone="danger">{error}</Notice>}
      </div>
      {confirmation}
    </>
  );
}
