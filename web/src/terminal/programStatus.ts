import type { components } from "../api/schema";
import type { TerminalSession } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { sessionShowsDestination } from "./sessionStatus";

// What a program inside the terminal said it is doing (OSC 7501), as the engine
// keeps it: the record that most needs the user.
export type ProgramStatus = components["schemas"]["TerminalProgramStatus"];

// ShownProgramStatus is a status a row shows. An idle program shows nothing.
export type ShownProgramStatus = ProgramStatus & { state: Exclude<ProgramStatus["state"], "idle"> };

// SeenProgramStatus is, per session, the newest program status version this
// browser has had on screen with the session. A done or failed result newer
// than that has not been seen.
export type SeenProgramStatus = ReadonlyMap<string, number>;

// rowProgramStatus is the program status a session's row shows in place of its
// destination, or null. A session that is connecting, has a problem or has ended
// shows that instead. Working and blocked show while they last; done and error
// until the session has been on screen since; idle never.
export function rowProgramStatus(session: TerminalSession, seen: SeenProgramStatus): ShownProgramStatus | null {
  const status = session.programStatus;
  if (status === undefined || !sessionShowsDestination(session)) return null;
  switch (status.state) {
    case "working":
    case "blocked":
      return { ...status, state: status.state };
    case "done":
    case "error":
      return status.version > (seen.get(session.id) ?? 0) ? { ...status, state: status.state } : null;
    case "idle":
      return null;
  }
}

const blockedLabels: Record<NonNullable<ProgramStatus["kind"]> | "none", MessageKey> = {
  permission: "terminal.programStatusPermission",
  question: "terminal.programStatusQuestion",
  auth: "terminal.programStatusAuth",
  none: "terminal.programStatusBlocked",
};

function programStatusLabel(t: Translate, status: ShownProgramStatus): string {
  switch (status.state) {
    case "blocked":
      return t(blockedLabels[status.kind ?? "none"]);
    case "working":
      return status.progress === undefined
        ? t("terminal.programStatusWorking")
        : t("terminal.programStatusWorkingPercent", { percent: String(status.progress) });
    case "done":
      return t("terminal.programStatusDone");
    case "error":
      return t("terminal.programStatusFailed");
  }
}

// programStatusText is the row's second line: the state, then what the program
// said about it (its message, or its title when it sent none).
export function programStatusText(t: Translate, status: ShownProgramStatus): string {
  const label = programStatusLabel(t, status);
  const said = status.message ?? status.title;
  return said === undefined ? label : `${label} · ${said}`;
}

const tones: Record<ShownProgramStatus["state"], string> = {
  blocked: "text-notice-ink",
  working: "text-accent",
  done: "text-live",
  error: "text-danger",
};

export function programStatusTone(status: ShownProgramStatus): string {
  return tones[status.state];
}
