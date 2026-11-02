import type { TerminalSession } from "../api/terminalSessions";
import type { Translate } from "../i18n/context";
import { endedRightAway } from "./sessionStatus";

// exitCodeUnknown is the engine's code for an exit whose status it could not learn (terminal.ExitCodeUnknown).
const exitCodeUnknown = -1;

// exitText says how the program of an ended session ended, in a sentence.
export function exitText(t: Translate, exited: NonNullable<TerminalSession["exited"]>, session: TerminalSession): string {
  if (exited.connectionLost === true) return t("terminal.exitedWithConnectionLost");
  if (exited.signal !== "") return t("terminal.exitedWithSignal", { signal: exited.signal });
  if (exited.code === 0 && endedRightAway(session)) return t("terminal.exitedWithCodeRightAway");
  if (exited.code === exitCodeUnknown) return t("terminal.exitedWithUnknownCode");
  return t("terminal.exitedWithCode", { code: String(exited.code) });
}
