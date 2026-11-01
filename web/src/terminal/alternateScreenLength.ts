import type { IDisposable, Terminal } from "@xterm/xterm";

// The part of xterm.js's own buffers this reaches into. It is not in xterm.js's API.
type InternalLines = { readonly length: number; maxLength: number };
type InternalTerminal = { _core?: { buffers?: { alt?: { lines?: InternalLines } } } };

// xterm.js (6.1.0-beta.315, and its main branch in November 2026) lowers a buffer's length limit only while the buffer
// holds lines: Buffer.resize does it inside `if (this.lines.length > 0)`. The alternate screen is emptied each time a
// program leaves it and made again with the rows of that moment. When the terminal is then made shorter (a phone's
// keyboard comes up) and the next program (vim, less, Claude Code) enters the alternate screen at that size, the
// screen is shorter than its limit: the lines that scroll off its top are kept as if it had a scrollback. Once the
// terminal is taller again, the screen starts that many lines down a buffer too short to hold it, and the program's
// redraw lands in the wrong rows.
//
// fitEmptyAlternateScreen lowers that limit to the rows whenever the terminal is resized while the alternate screen
// is empty. When xterm.js fixes Buffer.resize, the test that shows the fault (alternateScreenLength.test.ts) fails,
// and this goes.
export function fitEmptyAlternateScreen(view: Terminal): IDisposable {
  const fit = (rows: number) => {
    const lines = (view as unknown as InternalTerminal)._core?.buffers?.alt?.lines;
    if (lines === undefined || lines.length !== 0 || lines.maxLength <= rows) return;
    lines.maxLength = rows;
  };
  fit(view.rows);
  return view.onResize(({ rows }) => fit(rows));
}
