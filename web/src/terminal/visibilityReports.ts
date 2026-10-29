import type { IDisposable } from "@xterm/xterm";
import type { CSIParser } from "./escapeSequenceParser";

// Visibility reports (mode 2033) are answered by the sshc engine, which knows
// every browser showing the session (internal/terminal/visibility_reports.go).
// This browser says whether it shows the terminal; see terminalVisibility.ts.

// The mode visibility reports turn on, and the DECRQM question about it.
const visibilityReportsMode = 2033;

// attachVisibilityReportQuestions keeps xterm.js from answering DECRQM 2033
// itself: it would say it does not know the mode, and the program would read
// that next to the engine's answer.
export function attachVisibilityReportQuestions(parser: CSIParser): IDisposable {
  return parser.registerCsiHandler({ prefix: "?", intermediates: "$", final: "p" }, (params) => params[0] === visibilityReportsMode);
}
