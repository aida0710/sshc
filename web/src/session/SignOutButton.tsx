import { useLanguage } from "../i18n/context";
import { Icon } from "../ui/icons";
import { Notice } from "../ui/surface";
import { useSignOutConfirmation } from "./useSignOutConfirmation";

// SignOutButton is the lock screen's way out, so a browser can be signed out without knowing the
// master password: a quiet red button at the foot of the card, with what signing out does under it.
// The Menu draws its own row for the same.
export function SignOutButton() {
  const { t } = useLanguage();
  const { requestSignOut, busy, error, confirmation } = useSignOutConfirmation();
  return (
    <>
      <button type="button" disabled={busy} onClick={requestSignOut}
        className="inline-flex items-center gap-1.5 rounded-md text-xs font-medium text-danger hover:underline disabled:opacity-60">
        <Icon name="signOut" className="size-3.5" />
        {t("menu.signOut")}
      </button>
      <p className="mt-1 text-[11.5px] leading-[17px] text-ink-faint">{t("menu.signOutHint")}</p>
      {error === "" ? null : <div className="mt-2"><Notice tone="danger" compact>{error}</Notice></div>}
      {confirmation}
    </>
  );
}
