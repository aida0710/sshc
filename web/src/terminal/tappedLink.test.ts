import { Terminal } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import { linkAtPoint } from "./tappedLink";

const cellWidth = 10;
const cellHeight = 20;

async function tappable(output: string, cols = 30, rows = 4) {
  const terminal = new Terminal({ cols, rows, allowProposedApi: true });
  await new Promise<void>((resolve) => terminal.write(output, resolve));
  const element = document.createElement("div");
  const screen = document.createElement("div");
  screen.className = "xterm-screen";
  screen.getBoundingClientRect = () => new DOMRect(100, 50, cols * cellWidth, rows * cellHeight);
  element.appendChild(screen);
  const view = { element, rows, cols, buffer: terminal.buffer, options: terminal.options };
  return { terminal, view };
}

// The point in the middle of a 1-based cell.
function cell(column: number, row: number) {
  return { clientX: 100 + (column - 0.5) * cellWidth, clientY: 50 + (row - 0.5) * cellHeight };
}

describe("linkAtPoint", () => {
  it("URLを押すとそのURLを返す", async () => {
    const { terminal, view } = await tappable("see https://example.test/a now");
    try {
      expect(linkAtPoint(view, cell(10, 1), false)).toMatchObject({ kind: "url", target: "https://example.test/a" });
    } finally { terminal.dispose(); }
  });

  it("URLの外を押しても何も返さない", async () => {
    const { terminal, view } = await tappable("see https://example.test/a now");
    try {
      expect(linkAtPoint(view, cell(2, 1), false)).toBeNull();
      expect(linkAtPoint(view, cell(28, 1), false)).toBeNull();
      expect(linkAtPoint(view, cell(10, 2), false)).toBeNull();
    } finally { terminal.dispose(); }
  });

  it("折り返したURLは次の行を押しても同じURLを返す", async () => {
    const url = "https://example.test/a-long-path-that-wraps";
    const { terminal, view } = await tappable(url);
    try {
      expect(linkAtPoint(view, cell(3, 2), false)).toMatchObject({ target: url });
    } finally { terminal.dispose(); }
  });

  it("画面の外を押しても何も返さない", async () => {
    const { terminal, view } = await tappable("https://example.test/a");
    try {
      expect(linkAtPoint(view, { clientX: 90, clientY: 60 }, false)).toBeNull();
      expect(linkAtPoint(view, cell(5, 5), false)).toBeNull();
    } finally { terminal.dispose(); }
  });
});
