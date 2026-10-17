import { Unicode11Addon } from "@xterm/addon-unicode11";
import type { Terminal } from "@xterm/xterm";

// xterm.js counts character widths with Unicode 6 unless told otherwise, so an
// emoji takes one cell while the program that printed it (wcwidth in current
// C libraries, string-width in Node) counted two, and the rest of the line
// lands a column off. Unicode 11 matches what those programs count.
export function applyUnicode11Widths(view: Terminal): void {
  view.loadAddon(new Unicode11Addon());
  view.unicode.activeVersion = "11";
}
