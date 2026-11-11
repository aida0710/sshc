import type { IDisposable } from "@xterm/xterm";
import type { OSCParser } from "./escapeSequenceParser";

// OSC 7501 is the Program Status Protocol: a program such as Claude Code says it is working, waits for the person
// or is done. The engine keeps what it said (internal/terminal/program_status.go) and the session list carries it,
// so the browser does not read the report itself: attachProgramStatusReports only tells that one went by.
export function attachProgramStatusReports(parser: OSCParser, reported: () => void): IDisposable {
  return parser.registerOscHandler(7501, () => {
    reported();
    return true;
  });
}
