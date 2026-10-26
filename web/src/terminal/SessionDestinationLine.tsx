import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { sessionShowsDestination, sessionStatusText, sessionStatusTone } from "./sessionStatus";
import { terminalSubtitle } from "./terminalPresentation";

// The second line of a session row, under its name: where the session connects while it is connected,
// and its state in words otherwise ("reconnecting 2/5", "exited 255", ...).
export function SessionDestinationLine({ session }: { session: TerminalSession }) {
  const t = useTranslate();
  if (sessionShowsDestination(session)) {
    return <span data-session-destination className="block truncate font-mono text-[10.5px] leading-[14px] text-ink-faint">{terminalSubtitle(session, t)}</span>;
  }
  return <span data-session-status-text className={`block truncate text-[10.5px] leading-[14px] ${sessionStatusTone(session)}`}>{sessionStatusText(t, session)}</span>;
}
