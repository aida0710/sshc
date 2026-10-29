import type { IDisposable } from "@xterm/xterm";
import type { OSCParser } from "./escapeSequenceParser";

// attachOSC133Commands follows the shell integration's command marks. The
// engine times commands and notifies the long ones; the view needs only where
// a command starts and that it ended.
export function attachOSC133Commands(
  parser: OSCParser,
  callbacks: {
    onCommandStarted?: () => void;
    onCommandCompleted?: () => void;
  },
): IDisposable {
  let running = false;
  return parser.registerOscHandler(133, (data) => {
    const [kind] = data.split(";", 1);
    if (kind === "C") {
      running = true;
      callbacks.onCommandStarted?.();
      return true;
    }
    if (kind === "D") {
      if (running) callbacks.onCommandCompleted?.();
      running = false;
      return true;
    }
    // A (prompt start) and B (prompt end) are valid shell-integration markers.
    // Consume them so they never appear as text, while C/D define the command
    // region used by sshc.
    return kind === "A" || kind === "B";
  });
}
