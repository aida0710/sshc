import type { IDisposable } from "@xterm/xterm";
import type { OSCParser } from "./escapeSequenceParser";
import type { components } from "../api/schema";

// What a program reported through ConEmu's OSC 9;4, which Windows Terminal,
// Ghostty and WezTerm also show. "none" means nothing to show.
export type TerminalProgress =
  | { state: "none" }
  | components["schemas"]["TerminalProgramProgress"];

export const noTerminalProgress: TerminalProgress = { state: "none" };

const reportedStates = { 1: "normal", 2: "error", 4: "paused" } as const;

// nextTerminalProgress applies one `OSC 9 ; 4 ; state ; percent` payload (the
// text after "9;"). It returns null for any other OSC 9, such as an iTerm2
// notification or OSC 9;9. Error and paused may omit the percent and then keep
// the previous one, as in Windows Terminal.
export function nextTerminalProgress(previous: TerminalProgress, data: string): TerminalProgress | null {
  const [command, state = "", value = ""] = data.split(";");
  if (command !== "4" || !/^[0-4]$/.test(state)) return null;
  if (state === "0") return noTerminalProgress;
  if (state === "3") return { state: "indeterminate" };
  const parsed = /^\d{1,3}$/.test(value) ? Math.min(100, Number(value)) : null;
  const kept = previous.state === "normal" || previous.state === "error" || previous.state === "paused" ? previous.percent : 0;
  if (parsed === null && state === "1") return null;
  return { state: reportedStates[Number(state) as 1 | 2 | 4], percent: parsed ?? kept };
}

export function attachOSC9Progress(parser: OSCParser, changed: (update: (previous: TerminalProgress) => TerminalProgress) => void): IDisposable {
  return parser.registerOscHandler(9, (data) => {
    if (!data.startsWith("4;")) return false;
    changed((previous) => nextTerminalProgress(previous, data) ?? previous);
    return true;
  });
}
