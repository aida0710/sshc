import type { TerminalSession } from "../../api/terminalSessions";
import { useTranslate } from "../../i18n/context";
import { Icon } from "../../ui/icons";
import { ModalShell } from "../../ui/ModalShell";
import { terminalDisplayTitle } from "../terminalPresentation";

export function SplitSessionPicker({ sessions, onSelect, onOpenNew, onClose }: {
  sessions: TerminalSession[];
  onSelect: (sessionId: string) => void;
  // onOpenNew goes on to the choice of a new session; left out while no new pane fits.
  onOpenNew?: () => void;
  onClose: () => void;
}) {
  const t = useTranslate();
  return (
    <ModalShell labelledBy="split-session-heading" onDismiss={onClose} panelClassName="flex w-full max-w-sm flex-col gap-3 p-4">
      <h2 id="split-session-heading" className="text-sm font-semibold text-ink">{t("splitView.chooseSession")}</h2>
      <ul className="flex max-h-72 flex-col gap-1 overflow-y-auto">
        {sessions.map((session) => (
          <li key={session.id}><button type="button" className="w-full rounded-md px-2 py-2 text-left text-sm hover:bg-hover focus:bg-select-fill" onClick={() => onSelect(session.id)}>{terminalDisplayTitle(session)}</button></li>
        ))}
        {onOpenNew === undefined ? null : (
          <li>
            <button type="button" className="flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm text-accent hover:bg-hover focus:bg-select-fill" onClick={onOpenNew}>
              <Icon name="plus" className="size-3.5" aria-hidden="true" />
              {t("terminal.newSession")}
            </button>
          </li>
        )}
      </ul>
      <button type="button" className="self-end rounded-md px-2 py-1 text-sm text-ink-muted hover:bg-hover" onClick={onClose}>{t("splitView.cancel")}</button>
    </ModalShell>
  );
}
