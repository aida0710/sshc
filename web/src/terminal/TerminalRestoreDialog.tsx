import { useState } from "react";
import type { TerminalRestoreSession } from "../api/terminalRestore";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";

// The list names this many sessions; the rest are counted.
const listedSessionLimit = 5;

// TerminalRestoreDialog asks, once after the engine started again, whether to
// open the sessions that were open when it stopped. Declining ends the offer:
// the engine does not ask about the same sessions again. Escape or a tap
// outside only closes the question; it comes back the next time the screen
// loads.
export function TerminalRestoreDialog({
  sessions,
  host,
  onRestore,
  onDecline,
  onDismiss,
}: {
  sessions: readonly TerminalRestoreSession[];
  // host names the run target whose engine stopped; null for this machine.
  host: string | null;
  onRestore: () => Promise<unknown>;
  onDecline: () => void;
  onDismiss: () => void;
}) {
  const t = useTranslate();
  const [error, setError] = useState("");
  const listed = sessions.slice(0, listedSessionLimit);
  const unlisted = sessions.length - listed.length;

  async function restore() {
    setError("");
    try {
      await onRestore();
    } catch {
      setError(t("terminal.restoreFailed"));
    }
  }

  return (
    <ConfirmDialog
      id="terminal-restore"
      heading={t("terminal.restoreHeading")}
      body={
        <>
          <p className="text-sm text-ink-muted">{host === null
            ? t("terminal.restoreBody", { count: sessions.length })
            : t("terminal.restoreBodyRemote", { count: sessions.length, host })}</p>
          <ul className="flex flex-col gap-1.5 text-sm">
            {listed.map((session, index) => (
              <li key={index} className="flex min-w-0 flex-col">
                <span className="truncate text-ink">{session.pinnedTitle ?? session.alias ?? t("terminal.restoreLocalShell")}</span>
                {session.workingDirectory === undefined ? null : (
                  // A long path is cut at its start, so the folder it ends in stays readable.
                  <span dir="rtl" className="truncate text-left font-mono text-xs text-ink-muted">
                    <bdi dir="ltr">{session.workingDirectory}</bdi>
                  </span>
                )}
              </li>
            ))}
          </ul>
          {unlisted > 0 ? <p className="text-sm text-ink-muted">{t("terminal.restoreMore", { count: unlisted })}</p> : null}
        </>
      }
      confirmKind="primary"
      confirmLabel={t("terminal.restoreConfirm")}
      cancelLabel={t("terminal.restoreDecline")}
      onConfirm={restore}
      onCancel={onDecline}
      onDismiss={onDismiss}
      error={error}
    />
  );
}
