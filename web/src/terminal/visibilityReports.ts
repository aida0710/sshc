type Disposable = { dispose(): void };
type CsiParser = {
  registerCsiHandler(id: { prefix?: string; intermediates?: string; final: string }, handler: (params: (number | number[])[]) => boolean): Disposable;
};

// Visibility reports (mode 2033) are answered by the sshc engine, which knows
// every browser showing the session (internal/terminal/visibility_reports.go).
// This browser says whether it shows the terminal; see terminalVisibility.ts.

// The mode visibility reports turn on, and the DECRQM question about it.
const visibilityReportsMode = 2033;

// attachVisibilityReportQuestions keeps xterm.js from answering DECRQM 2033
// itself: it would say it does not know the mode, and the program would read
// that next to the engine's answer.
export function attachVisibilityReportQuestions(parser: CsiParser): Disposable {
  return parser.registerCsiHandler({ prefix: "?", intermediates: "$", final: "p" }, (params) => params[0] === visibilityReportsMode);
}
