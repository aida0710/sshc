import type { TerminalSession } from "../api/terminalSessions";
import { useSessionDestination } from "./sessionDestination";
import { sessionMarkerClass } from "./sessionStatus";
import { terminalDisplayTitle } from "./terminalPresentation";

// SessionNamesList says which sessions a count is about: each one's state, name and destination.
export function SessionNamesList({ sessions, label }: { sessions: TerminalSession[]; label: string }) {
  return (
    // A long list scrolls, so it takes focus for the keyboard to scroll it.
    <ul aria-label={label} tabIndex={0} className="max-h-40 overflow-y-auto rounded-md border border-line">
      {sessions.map((session) => <SessionNameRow key={session.id} session={session} />)}
    </ul>
  );
}

function SessionNameRow({ session }: { session: TerminalSession }) {
  const destination = useSessionDestination(session);
  return (
    <li className="flex items-center gap-2 border-line px-2.5 py-1.5 text-sm [&+&]:border-t">
      <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${sessionMarkerClass(session, false)}`} />
      <span title={terminalDisplayTitle(session)} className="min-w-0 truncate text-ink">{terminalDisplayTitle(session)}</span>
      <span className="ml-auto shrink-0 text-xs text-ink-muted">{destination}</span>
    </li>
  );
}
