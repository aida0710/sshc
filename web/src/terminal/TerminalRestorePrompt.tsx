import type { TerminalRestoreApi } from "../api/terminalRestore";
import { useTranslate } from "../i18n/context";
import { useToast } from "../ui/useToast";
import { TerminalRestoreDialog } from "./TerminalRestoreDialog";
import { useTerminalRestoreOffer } from "./useTerminalRestoreOffer";

// TerminalRestorePrompt asks about the sessions from before the engine in use
// last stopped: this machine's, or the run target's (host). It is mounted while
// that engine lists its sessions, so it asks once its Vault is unlocked (when
// SSH sessions can connect), and the reopened sessions appear in the list shown.
export function TerminalRestorePrompt({
  api,
  host,
  onRestored,
}: {
  api: TerminalRestoreApi;
  host: string | null;
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

  return <TerminalRestoreDialog sessions={offer.sessions} host={host} onRestore={restore} onDecline={decline} onDismiss={offer.dismiss} />;
}
