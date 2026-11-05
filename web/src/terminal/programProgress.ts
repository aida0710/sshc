import type { TerminalSession } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { sessionShowsDestination } from "./sessionStatus";

// ProgramProgress is a program's OSC 9;4 progress as the session list carries it; the list leaves it out once cleared.
export type ProgramProgress = NonNullable<TerminalSession["programProgress"]>;

// rowProgramProgress is the progress a session's row shows, or null. A session that is connecting, has a problem or
// has ended shows that instead, as it does in place of a program status (rowProgramStatus).
export function rowProgramProgress(session: TerminalSession): ProgramProgress | null {
  return session.programProgress !== undefined && sessionShowsDestination(session) ? session.programProgress : null;
}

const progressNames: Record<ProgramProgress["state"], MessageKey> = {
  normal: "terminal.programProgress",
  indeterminate: "terminal.programProgress",
  error: "terminal.programProgressError",
  paused: "terminal.programProgressPaused",
};

// programProgressName is the name of the header's bar; the bar's own value says how far the program has got.
export function programProgressName(t: Translate, progress: ProgramProgress): string {
  return t(progressNames[progress.state]);
}

const progressPercentTexts: Record<Exclude<ProgramProgress["state"], "indeterminate">, MessageKey> = {
  normal: "terminal.programProgressPercent",
  error: "terminal.programProgressErrorPercent",
  paused: "terminal.programProgressPausedPercent",
};

// programProgressText says the progress in words with its percent, for the row's description: the ring at the start
// of the row is drawn for the eye only.
export function programProgressText(t: Translate, progress: ProgramProgress): string {
  if (progress.state === "indeterminate") return t("terminal.programProgress");
  return t(progressPercentTexts[progress.state], { percent: String(progress.percent) });
}
