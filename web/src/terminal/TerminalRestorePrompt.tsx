import { terminalRestoreApi, type TerminalRestoreApi } from "../api/terminalRestore";
import { useTranslate } from "../i18n/context";
import { useToast } from "../ui/useToast";
import { TerminalRestoreDialog } from "./TerminalRestoreDialog";
import { useTerminalRestoreOffer } from "./useTerminalRestoreOffer";

// TerminalRestorePrompt asks about the sessions from before the engine's last
// stop. It is mounted while the screen is usable, so it asks again after the
// Vault is unlocked, which is also when SSH sessions can connect.
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

  return <TerminalRestoreDialog sessions={offer.sessions} onRestore={restore} onDecline={decline} />;
}
