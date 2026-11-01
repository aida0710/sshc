import type { IBufferRange, Terminal } from "@xterm/xterm";
import { modifierOpensLink, type TerminalLinkMatch } from "./links";
import { findBufferLinks } from "./bufferLinks";

// Offers the URLs and paths found in the buffer to the mouse. A modifier-click opens a URL directly; a plain
// click hands the match to the caller for a popover. xterm draws no line under the pointed link: point hands
// its range over, and the caller underlines it as a touch terminal underlines its links (linkUnderlines.ts).
export function attachLinkProvider(view: Terminal, {
  open,
  select,
  point,
  allowWindowsPaths = () => false,
}: {
  allowWindowsPaths?: () => boolean;
  open: (target: string) => void;
  select: (link: TerminalLinkMatch, event: MouseEvent) => void;
  // point follows the pointer onto a link (its range) and off it (null).
  point: (range: IBufferRange | null) => void;
}): { dispose(): void } {
  return view.registerLinkProvider({
    provideLinks: (bufferLineNumber, callback) => {
      const matches = findBufferLinks(view.buffer.active, bufferLineNumber, allowWindowsPaths());
      callback(matches.length === 0 ? undefined : matches.map(({ match, range }) => ({
        text: match.text,
        range,
        decorations: { pointerCursor: true, underline: false },
        hover: () => point(range),
        leave: () => point(null),
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
