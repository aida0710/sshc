import { terminalRestoreApi, type TerminalRestoreApi } from "../api/terminalRestore";
import { useTranslate } from "../i18n/context";
import { useToast } from "../ui/useToast";
import { TerminalRestoreDialog } from "./TerminalRestoreDialog";
import { useTerminalRestoreOffer } from "./useTerminalRestoreOffer";

// TerminalRestorePrompt asks about the sessions from before this machine's
// engine last stopped. It is mounted while the screen is usable and this
// machine is the run target, so it asks after the Vault is unlocked (when SSH
// sessions can connect) and the reopened sessions appear in the list shown.
export function TerminalRestorePrompt({
  api = terminalRestoreApi,
  onRestored,
}: {
  api?: TerminalRestoreApi;
  onRestored: () => Promise<void>;
}) {
  const t = useTranslate();
  const showToast = useToast();
  const offer = useTerminalRestoreOffer(api);
  if (offer.sessions.length === 0) return null;

  async function restore() {
    const result = await offer.restore();
    await onRestored();
    if (result.failed > 0) showToast(t("terminal.restorePartlyFailed", { count: result.failed }));
  }

  function decline() {
    offer.decline().catch(() => showToast(t("terminal.restoreFailed")));
  }

  return <TerminalRestoreDialog sessions={offer.sessions} onRestore={restore} onDecline={decline} onDismiss={offer.dismiss} />;
}
