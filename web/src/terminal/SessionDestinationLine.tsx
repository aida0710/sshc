import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { programStatusText, programStatusTone, type ShownProgramStatus } from "./programStatus";
import { sessionShowsDestination, sessionStatusText, sessionStatusTone } from "./sessionStatus";
import { terminalSubtitle } from "./terminalPresentation";

// The second line of a session row, under its name: where the session connects while it is connected,
// and its state in words otherwise ("reconnecting 2/5", "exited 255", ...). While the row shows what the
// program inside said it is doing (rowProgramStatus), that takes the destination's place.
// id lets a row's button read the line as its description, since the button's name is the session's.
export function SessionDestinationLine({ session, programStatus = null, id }: {
  session: TerminalSession;
  programStatus?: ShownProgramStatus | null;
  id?: string;
}) {
  const t = useTranslate();
  if (programStatus !== null) {
    return <span id={id} data-program-status-text className={`block truncate text-[10.5px] leading-[14px] ${programStatusTone(programStatus)}`}>{programStatusText(t, programStatus)}</span>;
  }
  if (sessionShowsDestination(session)) {
    return <span id={id} data-session-destination className="block truncate font-mono text-[10.5px] leading-[14px] text-ink-faint">{terminalSubtitle(session, t)}</span>;
  }
  return <span id={id} data-session-status-text className={`block truncate text-[10.5px] leading-[14px] ${sessionStatusTone(session)}`}>{sessionStatusText(t, session)}</span>;
}
