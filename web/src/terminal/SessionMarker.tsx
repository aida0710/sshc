import type { CSSProperties } from "react";
import type { TerminalSession } from "../api/terminalSessions";
import type { ProgramProgress } from "./programProgress";
import type { ShownProgramStatus } from "./programStatus";
import { useSharedTurn } from "../ui/useSharedTurn";
import { sessionMarkerClass } from "./sessionStatus";

// The connection dot takes the size of the program status marks, an 11px circle,
// so a row's name stays in place when a status takes the dot's place and the two
// look alike in the list. The circle is centred on the 18px name line. Where the
// letters sit in that line depends on the platform's font: measured, the circle is
// within 1px of their middle on Android and Windows, and only Linux at twice the
// pixel density draws the letters a pixel higher.
const markerCircle = "mt-[3.5px] size-[11px] shrink-0 rounded-full";

// The mark that stands for each program status that needs the person: a filled
// circle with a sign once it waits on them, failed or is done.
const programStatusMarks: Record<Exclude<ShownProgramStatus["state"], "working">, { fill: string; sign: string }> = {
  blocked: { fill: "bg-notice-ink shadow-[0_0_0_3px_color-mix(in_srgb,var(--ui-notice-ink)_22%,transparent)]", sign: "!" },
  error: { fill: "bg-danger", sign: "×" },
  done: { fill: "bg-live", sign: "✓" },
};

// The progress ring takes the colors of the bar under the pane header (TerminalProgressBar).
const progressRingColors: Record<Exclude<ProgramProgress["state"], "indeterminate">, string> = {
  normal: "text-accent",
  error: "text-danger",
  paused: "text-notice-ink",
};

// SessionMarker is the mark at the start of a session's row. What the program
// in it says takes the place of the dot that says whether the session is
// connected: first a program status that needs the person (OSC 7501), then how
// far the program has got (OSC 9;4) as a ring filled that far, then a turning
// ring while it works without saying how far. The row's status text says the
// same for screen readers, so the mark is hidden from them.
export function SessionMarker({ session, unread, programStatus, programProgress }: {
  session: TerminalSession;
  unread: boolean;
  programStatus: ShownProgramStatus | null;
  programProgress: ProgramProgress | null;
}) {
  if (programStatus !== null && programStatus.state !== "working") {
    const mark = programStatusMarks[programStatus.state];
    return (
      <span aria-hidden="true" data-program-status={programStatus.state}
        className={`${markerCircle} flex items-center justify-center text-[8px] font-bold leading-none text-canvas ${mark.fill}`}>
        {mark.sign}
      </span>
    );
  }
  if (programProgress !== null && programProgress.state !== "indeterminate") {
    return <span aria-hidden="true" data-program-progress={programProgress.state}
      className={`${markerCircle} sshc-progress-ring ${progressRingColors[programProgress.state]}`}
      style={{ "--sshc-progress": `${programProgress.percent}%` } as CSSProperties} />;
  }
  if (programStatus !== null || programProgress !== null) {
    return <TurningRing source={programStatus === null ? { "data-program-progress": "indeterminate" } : { "data-program-status": "working" }} />;
  }
  return <span aria-hidden="true" className={`${markerCircle} ${sessionMarkerClass(session, unread)}`} />;
}

// TurningRing is the ring that turns while a program works without saying how far. It is a component of its own so
// that it is set in step with the other rows' rings when it appears, whenever its program began.
function TurningRing({ source }: { source: { "data-program-progress": "indeterminate" } | { "data-program-status": "working" } }) {
  const ring = useSharedTurn<HTMLSpanElement>();
  return <span ref={ring} aria-hidden="true" {...source} className={`${markerCircle} animate-spin border-2 border-accent/25 border-t-accent`} />;
}
