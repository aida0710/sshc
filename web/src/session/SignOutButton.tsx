import { useLanguage } from "../i18n/context";
import { Notice } from "../ui/surface";
import { useSignOutConfirmation } from "./useSignOutConfirmation";

// SignOutButton is the lock screen's way out, so a browser can be signed out without knowing the
// master password: a quiet link at the foot of the card. What signing out does is said when it is
// confirmed, not under the link, where it would be read on every unlock. The Menu draws its own row for
// the same.
export function SignOutButton() {
  const { t } = useLanguage();
  const { requestSignOut, busy, error, confirmation } = useSignOutConfirmation();
  return (
    <>
      <button type="button" disabled={busy} onClick={requestSignOut}
        className="rounded-md text-xs text-ink-faint hover:text-ink hover:underline disabled:opacity-60">
        {t("menu.signOut")}
      </button>
      {error === "" ? null : <div className="mt-2"><Notice tone="danger" compact>{error}</Notice></div>}
      {confirmation}
    </>
  );
}
