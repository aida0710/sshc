import type { Terminal } from "@xterm/xterm";
import { findTerminalLinks, modifierOpensLink, type TerminalLinkMatch } from "./links";

// Underlines URLs and paths found in the buffer. A modifier-click opens a
// URL directly; a plain click hands the match to the caller for a popover.
export function attachLinkProvider(view: Terminal, {
  remote,
  open,
  select,
}: {
  remote: boolean;
  open: (target: string) => void;
  select: (link: TerminalLinkMatch, event: MouseEvent) => void;
}): { dispose(): void } {
  return view.registerLinkProvider({
    provideLinks: (bufferLineNumber, callback) => {
      const line = view.buffer.active.getLine(bufferLineNumber - 1)?.translateToString(true) ?? "";
      const matches = findTerminalLinks(line, remote);
      callback(matches.length === 0 ? undefined : matches.map((match) => ({
        text: match.text,
        range: {
          start: { x: match.start + 1, y: bufferLineNumber },
          end: { x: match.end, y: bufferLineNumber },
        },
        activate: (event: MouseEvent) => {
          if (match.kind === "url" && modifierOpensLink(event)) {
            open(match.target);
            return;
          }
          select(match, event);
        },
      })));
    },
  });
}
