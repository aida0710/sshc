import type { Terminal } from "@xterm/xterm";
import { modifierOpensLink, type TerminalLinkMatch } from "./links";
import { findBufferLinks } from "./bufferLinks";

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
      const matches = findBufferLinks(view.buffer.active, bufferLineNumber, remote);
      callback(matches.length === 0 ? undefined : matches.map(({ match, range }) => ({
        text: match.text,
        range,
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
