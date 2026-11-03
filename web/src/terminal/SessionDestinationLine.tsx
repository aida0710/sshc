import type { TerminalSession } from "../api/terminalSessions";
import { useTranslate } from "../i18n/context";
import { programStatusText, programStatusTone, type ShownProgramStatus } from "./programStatus";
import { rowProblemText, sessionShowsDestination, sessionStatusText, sessionStatusTone } from "./sessionStatus";
import { useSessionDestination } from "./sessionDestination";

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
  const destination = useSessionDestination(session);
  if (programStatus !== null) {
    return <span id={id} data-program-status-text className={`block truncate text-[10.5px] leading-[14px] ${programStatusTone(programStatus)}`}>{programStatusText(t, programStatus)}</span>;
  }
  if (sessionShowsDestination(session)) {
    return <span id={id} data-session-destination className="block truncate font-mono text-[10.5px] leading-[14px] text-ink-faint">{destination}</span>;
  }
  // A problem is explained in sentences where the session is shown. The row has one short line under the name,
  // so it says the reason in a few words, and keeps the sentences for the tooltip.
  const status = sessionStatusText(t, session);
  const shown = session.problem === "" ? status : rowProblemText(t, session.problem);
  return (
    <span id={id} data-session-status-text title={shown === status ? undefined : status} className={`block truncate text-[10.5px] leading-[14px] ${sessionStatusTone(session)}`}>
      {shown}
    </span>
  );
}
