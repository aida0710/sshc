import { Terminal } from "@xterm/xterm";
import type { ILink, ILinkProvider } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import { findBufferLinks } from "./bufferLinks";
import { imagePathsInBuffer } from "./imagePaths";
import { attachLinkProvider } from "./linkProvider";

// Ink erases to the edge, returns to column one and moves down rather than
// letting xterm wrap. Real xterm cells keep isWrapped false for these rows.
const hardBreak = "\x1b[K\r\x1b[1B";

async function terminalWithRows(rows: string[], cols: number): Promise<Terminal> {
  const terminal = new Terminal({ cols, rows: Math.max(24, rows.length + 1), allowProposedApi: true });
  await new Promise<void>((resolve) => terminal.write(rows.join(hardBreak), resolve));
  return terminal;
}

function wrapPath(printed: string, cols: number, indent = "  "): string[] {
  const rows = [printed.slice(0, cols)];
  for (let offset = cols; offset < printed.length; offset += cols - indent.length) {
    rows.push(indent + printed.slice(offset, offset + cols - indent.length));
  }
  return rows;
}

describe("program-wrapped path links", () => {
  it("joins the recorded four-row and two-row paths when they fill the terminal width", async () => {
    const rows = [
      "● /tmp/claude-1000/-home-aida-projects-sshc/c1639c4f-2b77-47a4-a002-56368c81d6d2",
      "  /scratchpad/longpath/artifacts/terminal-path-preview-long-verification/2026-10",
      "  -23/screenshots-of-the-sidebar/workspace-sidebar-after-the-drag-and-drop-chang",
      "  e.png",
      "  artifacts/terminal-path-preview-long-verification/2026-10-23/screenshots-of-th",
      "  e-sidebar/workspace-sidebar-after-the-drag-and-drop-change.png",
    ];
    // Keep the captured 80-column geometry, including the bullet and indentation.
    const cols = 80;
    expect(rows.slice(0, 3).map((row) => row.length)).toEqual([cols, cols, cols]);
    expect(rows[4]!.length).toBe(cols);
    const terminal = await terminalWithRows(rows, cols);
    const absolute = rows.slice(0, 4).map((row) => row.slice(2)).join("");
    const relative = rows.slice(4).map((row) => row.slice(2)).join("");
    try {
      for (const [index, row] of rows.entries()) {
        expect(terminal.buffer.active.getLine(index)?.isWrapped).toBe(false);
        expect(findBufferLinks(terminal.buffer.active, index + 1)).toMatchObject([{
          match: { target: index < 4 ? absolute : relative },
          range: { start: { x: 3, y: index + 1 }, end: { x: row.length, y: index + 1 } },
        }]);
        expect(findBufferLinks(terminal.buffer.active, index + 1)).toHaveLength(1);
      }
      expect(imagePathsInBuffer(terminal.buffer.active, { directory: "/work", home: null })).toEqual([absolute, `/work/${relative}`]);
    } finally { terminal.dispose(); }
  });

  it.each([
    ["/tmp/long-folder/long-image-name.png", "/tmp/long-folder/long-image-name.png"],
    ["~/long-folder/long-image-name.png", "~/long-folder/long-image-name.png"],
    ["artifacts/long-folder/long-image-name.png", "artifacts/long-folder/long-image-name.png"],
    ["long-image-name-without-a-folder.png", "long-image-name-without-a-folder.png"],
    ['"a long-name-with-spaces-and-suffix.png"', "a long-name-with-spaces-and-suffix.png"],
    ["● long-image-name-without-a-folder.png", "long-image-name-without-a-folder.png"],
    ['"/tmp/long folder/long image name.png"', "/tmp/long folder/long image name.png"],
    ["'/tmp/long folder/long image name.png'", "/tmp/long folder/long image name.png"],
    ["`/tmp/long-folder/long-image-name.png`", "/tmp/long-folder/long-image-name.png"],
    ["/tmp/long\\ folder/long-image-name.png", "/tmp/long folder/long-image-name.png"],
    ["$HOME/long-folder/long-image-name.png", "~/long-folder/long-image-name.png"],
    ["${HOME}/long-folder/long-image-name.png", "~/long-folder/long-image-name.png"],
    ["file:///tmp/long%20folder/long-image-name.png", "/tmp/long folder/long-image-name.png"],
    ["Saved to:/tmp/long-folder/long-image-name.png:12:3", "/tmp/long-folder/long-image-name.png"],
    ["→/tmp/long-folder/long-image-name.png", "/tmp/long-folder/long-image-name.png"],
    ["C:\\Users\\aida\\long-folder\\long-image-name.png", "C:/Users/aida/long-folder/long-image-name.png"],
  ])("preserves the supported path notation %s across multiple rows", async (printed, target) => {
    const rows = wrapPath(printed, 18, "    ");
    const terminal = await terminalWithRows(rows, 18);
    try {
      for (let row = 1; row <= rows.length; row += 1) {
        expect(findBufferLinks(terminal.buffer.active, row, true).map(({ match }) => match.target)).toEqual([target]);
      }
    } finally { terminal.dispose(); }
  });

  it("joins paths at column 80 without including the indentation", async () => {
    const target = `/tmp/${"long-folder/".repeat(20)}image.png`;
    const rows = wrapPath(target, 80, "     ");
    const terminal = await terminalWithRows(rows, 80);
    try {
      for (let row = 1; row <= rows.length; row += 1) {
        expect(findBufferLinks(terminal.buffer.active, row)).toMatchObject([{
          match: { target },
          range: { start: { x: row === 1 ? 1 : 6, y: row }, end: { x: rows[row - 1]!.length, y: row } },
        }]);
      }
    } finally { terminal.dispose(); }
  });

  it("joins after a wide character that fills the last two cells", async () => {
    const terminal = await terminalWithRows(["/tmp/1234567890123界", "  image.png"], 20);
    try {
      for (const row of [1, 2]) {
        expect(findBufferLinks(terminal.buffer.active, row).map(({ match }) => match.target)).toEqual([
          "/tmp/1234567890123界image.png",
        ]);
      }
    } finally { terminal.dispose(); }
  });

  it("keeps wide characters and excludes the unused column before them", async () => {
    const rows = ["/tmp/12345678901234", "    界/画像.png"];
    const terminal = await terminalWithRows(rows, 20);
    try {
      expect(findBufferLinks(terminal.buffer.active, 1)).toMatchObject([{
        match: { target: "/tmp/12345678901234界/画像.png" },
        range: { start: { x: 1, y: 1 }, end: { x: 19, y: 1 } },
      }]);
      expect(findBufferLinks(terminal.buffer.active, 2)).toMatchObject([{
        match: { target: "/tmp/12345678901234界/画像.png" },
        range: { start: { x: 5, y: 2 }, end: { x: 15, y: 2 } },
      }]);
    } finally { terminal.dispose(); }
  });

  it.each([
    ["/tmp/short", "  separate.png", 20],
    ["/tmp/12345678901234 ", "  separate.png", 20],
    ["/tmp/12345678901234", "  界.png", 21],
    ["/tmp/123456789012345", "continuation.png", 20],
    ["/tmp/123456789012345", "  ● separate.png", 20],
    ["/tmp/123456789012345", "  ❯ separate.png", 20],
    ["/tmp/123456789012345", "                    ", 20],
    ["ordinary prose words", "  separate.png", 20],
    ["ordinary prose words", "  /tmp/separate.png", 20],
    ["/tmp/12345678901.png", "  separate.png", 20],
    ["https://example.test", "  image.png", 20],
  ])("does not join unrelated rows %s / %s", async (first, second, cols) => {
    const terminal = await terminalWithRows([first, second], cols);
    try {
      for (const row of [1, 2]) {
        const links = findBufferLinks(terminal.buffer.active, row);
        expect(links.every(({ range }) => range.start.y === row && range.end.y === row)).toBe(true);
        if (second.trim() !== "") expect(links.some(({ match }) => match.text.includes(first.trim()) && match.text.includes(second.trim()))).toBe(false);
      }
    } finally { terminal.dispose(); }
  });

  it("joins hard and automatic wraps in the same path", async () => {
    const terminal = await terminalWithRows(["/tmp/123456789012345", "    continued/automatically-wrapped/image.png"], 20);
    try {
      for (const row of [1, 2, 3, 4]) {
        expect(findBufferLinks(terminal.buffer.active, row).map(({ match }) => match.target)).toEqual([
          "/tmp/123456789012345continued/automatically-wrapped/image.png",
        ]);
      }
    } finally { terminal.dispose(); }
  });

  it("leaves an overlong chain unlinked instead of offering truncated paths", async () => {
    const rows = ["/tmp/123456789012345", ...Array<string>(32).fill("  " + "a".repeat(18)), "  end.png"];
    const terminal = await terminalWithRows(rows, 20);
    try {
      for (let row = 1; row <= rows.length; row += 1) expect(findBufferLinks(terminal.buffer.active, row)).toEqual([]);
    } finally { terminal.dispose(); }
  });

  it("selects the whole path when any continuation is clicked", async () => {
    const target = "/tmp/long-folder/long-image-name.png";
    const rows = wrapPath(target, 18);
    const terminal = await terminalWithRows(rows, 18);
    let provider: ILinkProvider | undefined;
    terminal.registerLinkProvider = (registered) => { provider = registered; return { dispose() {} }; };
    const selected: string[] = [];
    attachLinkProvider(terminal, { open: () => {}, select: (match) => selected.push(match.target) });
    try {
      for (let row = 1; row <= rows.length; row += 1) {
        const links = await new Promise<ILink[] | undefined>((resolve) => provider?.provideLinks(row, resolve));
        expect(links).toHaveLength(1);
        links![0]!.activate(new MouseEvent("click"), target);
      }
      expect(selected).toEqual(rows.map(() => target));
    } finally { terminal.dispose(); }
  });
});
