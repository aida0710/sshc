import type { TerminalSession } from "../../api/terminalSessions";
import { useTranslate } from "../../i18n/context";
import { ModalShell } from "../../ui/ModalShell";
import { terminalDisplayTitle } from "../terminalPresentation";

export function SplitSessionPicker({ sessions, onSelect, onClose }: {
  sessions: TerminalSession[];
  onSelect: (sessionId: string) => void;
  onClose: () => void;
}) {
  const t = useTranslate();
  return (
    <ModalShell labelledBy="split-session-heading" onDismiss={onClose} panelClassName="flex w-full max-w-sm flex-col gap-3 rounded-lg p-4">
      <h2 id="split-session-heading" className="text-sm font-medium text-ink">{t("splitView.chooseSession")}</h2>
      <ul className="flex max-h-72 flex-col gap-1 overflow-y-auto">
        {sessions.map((session) => (
          <li key={session.id}><button type="button" className="w-full rounded px-2 py-2 text-left text-sm hover:bg-hover focus:bg-select-fill" onClick={() => onSelect(session.id)}>{terminalDisplayTitle(session)}</button></li>
        ))}
      </ul>
      <button type="button" className="self-end rounded px-2 py-1 text-sm text-ink-muted hover:bg-hover" onClick={onClose}>{t("splitView.cancel")}</button>
    </ModalShell>
  );
}
