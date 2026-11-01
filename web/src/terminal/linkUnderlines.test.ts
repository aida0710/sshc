import { Terminal, type ILink, type ILinkProvider } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import { attachLinkProvider } from "./linkProvider";
import { linkSegmentsOnScreen, rangeSegments } from "./linkUnderlines";

async function terminalWithOutput(output: string, { cols = 30, rows = 24 } = {}): Promise<Terminal> {
  const terminal = new Terminal({ cols, rows, allowProposedApi: true });
  await new Promise<void>((resolve) => terminal.write(output, resolve));
  return terminal;
}

function segmentsOf(terminal: Terminal) {
  return linkSegmentsOnScreen(terminal.buffer.active, { rows: terminal.rows, cols: terminal.cols, allowWindowsPaths: false });
}

const url = "https://example.test/authorize?redirect_uri=http%3A%2F%2F127.0.0.1&state=abc123";

describe("the lines under every link on the screen", () => {
  it("underlines a wrapped URL once, on each row it is on", async () => {
    const terminal = await terminalWithOutput(url);
    try {
      expect(segmentsOf(terminal)).toEqual([
        { row: 0, column: 0, cells: 30 },
        { row: 1, column: 0, cells: 30 },
        { row: 2, column: 0, cells: url.length % 30 },
      ]);
    } finally { terminal.dispose(); }
  });

  it("places links after Japanese text by cells, and underlines nothing else", async () => {
    const terminal = await terminalWithOutput("保存 shots/a.png と https://a.test/\r\nplain words", { cols: 60 });
    try {
      // 保存 takes 4 cells and the space one more; と takes 2 between two spaces.
      expect(segmentsOf(terminal)).toEqual([
        { row: 0, column: 5, cells: "shots/a.png".length },
        { row: 0, column: 20, cells: "https://a.test/".length },
      ]);
    } finally { terminal.dispose(); }
  });

  it("underlines only the rows on the screen of a link that starts above it", async () => {
    // The URL takes buffer lines 0 to 2 and three short lines follow; five rows show lines 1 to 5.
    const terminal = await terminalWithOutput(`${url}\r\nb\r\nc\r\nd`, { rows: 5 });
    try {
      expect(terminal.buffer.active.viewportY).toBe(1);
      expect(segmentsOf(terminal)).toEqual([
        { row: 0, column: 0, cells: 30 },
        { row: 1, column: 0, cells: url.length % 30 },
      ]);
    } finally { terminal.dispose(); }
  });
});

describe("the line under the link the mouse is on", () => {
  it("is drawn by sshc from the range xterm hands over, not by xterm", async () => {
    const terminal = await terminalWithOutput("see https://a.test/ now");
    let provider: ILinkProvider | undefined;
    terminal.registerLinkProvider = (registered) => { provider = registered; return { dispose() {} }; };
    const pointed: unknown[] = [];
    attachLinkProvider(terminal, { open: () => {}, select: () => {}, point: (range) => pointed.push(range) });
    try {
      const links = await new Promise<ILink[] | undefined>((resolve) => provider?.provideLinks(1, resolve));
      const link = links![0]!;
      expect(link.decorations).toEqual({ pointerCursor: true, underline: false });
      link.hover!(new MouseEvent("mousemove"), link.text);
      link.leave!(new MouseEvent("mousemove"), link.text);
      const range = { start: { x: 5, y: 1 }, end: { x: 19, y: 1 } };
      expect(pointed).toEqual([range, null]);
      expect(rangeSegments(range, { top: 0, rows: terminal.rows, cols: terminal.cols })).toEqual([{ row: 0, column: 4, cells: 15 }]);
    } finally { terminal.dispose(); }
  });
});
