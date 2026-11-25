import { Terminal } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import { findBufferLinks } from "./bufferLinks";

// Ink erases to the edge, returns to column one and moves down rather than
// letting xterm wrap. Real xterm cells keep isWrapped false for these rows.
const hardBreak = "\x1b[K\r\x1b[1B";

async function terminalWithRows(rows: string[], cols: number): Promise<Terminal> {
  const terminal = new Terminal({ cols, rows: Math.max(24, rows.length + 1), allowProposedApi: true });
  await new Promise<void>((resolve) => terminal.write(rows.join(hardBreak), resolve));
  return terminal;
}

describe("program-wrapped URL links", () => {
  const cols = 40;
  const url = "https://example.test/authorize?redirect_uri=http%3A%2F%2F127.0.0.1%3A8080%2Fcallback&state=abc123";

  // Claude Code on a phone: a bullet, then the URL broken at the edge with the
  // message's two-space indentation. The text is ASCII so that characters and cells agree.
  function claudeRows(before: string, after: string): string[] {
    const printed = `${before}${url}`;
    const rows = [printed.slice(0, cols)];
    let offset = cols;
    while (offset < printed.length) {
      rows.push(`  ${printed.slice(offset, offset + cols - 2)}`);
      offset += cols - 2;
    }
    rows[rows.length - 1] += after;
    return rows;
  }

  it("opens the whole URL from every row a program broke it over", async () => {
    const rows = claudeRows("● Open: ", " to allow it.");
    expect(rows.slice(0, -1).map((row) => row.length)).toEqual(rows.slice(0, -1).map(() => cols));
    const terminal = await terminalWithRows(rows, cols);
    try {
      for (const index of rows.keys()) {
        expect(terminal.buffer.active.getLine(index)?.isWrapped).toBe(false);
        const links = findBufferLinks(terminal.buffer.active, index + 1);
        expect(links.map((link) => link.match.target)).toEqual([url]);
      }
    } finally { terminal.dispose(); }
  });

  // 幅が 45 桁のとき、Claude Code はこの URL を redirect_uri のあとで折り、続きの行は =http で始まる。
  it("opens the whole URL when a row it was broken over starts with a mark, as in =http or &state", async () => {
    for (const mark of ["=", "&", "?", "#", "+", ":", "@"]) {
      const head = "https://example.test/auth?redirect";
      const rows = [`  ${head}`.padEnd(cols, "x"), `  ${mark}http%3A%2F%2F127.0.0.1%2Fcallback`];
      const whole = `${rows[0]!.trim()}${rows[1]!.trim()}`;
      const terminal = await terminalWithRows(rows, cols);
      try {
        for (const index of rows.keys()) {
          expect(findBufferLinks(terminal.buffer.active, index + 1).map((link) => link.match.target), `続きが ${mark} で始まる行 ${index + 1}`).toEqual([whole]);
        }
      } finally { terminal.dispose(); }
    }
  });

  it("does not take a heading or a list item under a URL that ends at the edge for the rest of it", async () => {
    const short = "https://example.test/docs/install";
    const first = `${"  See:".padEnd(cols - short.length, " ")}${short}`;
    for (const next of ["  # Other systems", "  + other/systems", "  * other/systems"]) {
      const terminal = await terminalWithRows([first, next], cols);
      try {
        expect(findBufferLinks(terminal.buffer.active, 1).map((link) => link.match.target), next).toEqual([short]);
      } finally { terminal.dispose(); }
    }
  });

  it("does not glue the next word to a URL that only happens to end at the edge", async () => {
    const short = "https://example.test/docs/install";
    const first = `${"* See:".padEnd(cols - short.length, " ")}${short}`;
    expect(first).toHaveLength(cols);
    const terminal = await terminalWithRows([first, "  for the other systems."], cols);
    try {
      expect(findBufferLinks(terminal.buffer.active, 1).map((link) => link.match.target)).toEqual([short]);
      expect(findBufferLinks(terminal.buffer.active, 2)).toEqual([]);
    } finally { terminal.dispose(); }
  });
});
