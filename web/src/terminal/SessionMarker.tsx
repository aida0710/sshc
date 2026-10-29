import type { TerminalSession } from "../api/terminalSessions";
import type { ShownProgramStatus } from "./programStatus";
import { sessionMarkerClass } from "./sessionStatus";

// The mark that stands for each program status: a turning ring while it works,
// and a filled circle with a sign once it waits on the person, failed or is done.
const programStatusMarks: Record<Exclude<ShownProgramStatus["state"], "working">, { fill: string; sign: string }> = {
  blocked: { fill: "bg-notice-ink shadow-[0_0_0_3px_color-mix(in_srgb,var(--ui-notice-ink)_22%,transparent)]", sign: "!" },
  error: { fill: "bg-danger", sign: "×" },
  done: { fill: "bg-live", sign: "✓" },
};

// SessionMarker is the mark at the start of a session's row. The program's
// status, when the row shows one, takes the place of the dot that says whether
// the session is connected. The row's status text says the same for screen
// readers, so the mark is hidden from them.
export function SessionMarker({ session, unread, programStatus }: {
  session: TerminalSession;
  unread: boolean;
  programStatus: ShownProgramStatus | null;
}) {
  if (programStatus === null) {
    return <span aria-hidden="true" className={`mt-[5px] size-2 shrink-0 rounded-full ${sessionMarkerClass(session, unread)}`} />;
  }
  // 11px centred on the 18px name line.
  if (programStatus.state === "working") {
    return <span aria-hidden="true" data-program-status="working"
      className="mt-[3.5px] size-[11px] shrink-0 animate-spin rounded-full border-2 border-accent/25 border-t-accent" />;
  }
  const mark = programStatusMarks[programStatus.state];
  return (
    <span aria-hidden="true" data-program-status={programStatus.state}
      className={`mt-[3.5px] flex size-[11px] shrink-0 items-center justify-center rounded-full text-[8px] font-bold leading-none text-canvas ${mark.fill}`}>
      {mark.sign}
    </span>
  );
}
