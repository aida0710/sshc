import { Terminal } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import { drawEdgeRow, paletteColour } from "./edgeRow";

const theme = { foreground: "#dddddd", background: "#000000", red: "#ff0000", brightBlue: "#8888ff" };

function write(view: Terminal, data: string): Promise<void> {
  return new Promise((resolve) => view.write(data, resolve));
}

describe("drawEdgeRow", () => {
  it("draws each cell in a box of the cells' width, with its colour, and leaves out the blank end", async () => {
    const view = new Terminal({ cols: 20, rows: 3, allowProposedApi: true });
    await write(view, "\x1b[31mred\x1b[0m \x1b[1mbold\x1b[0m");
    const element = document.createElement("div");
    drawEdgeRow(element, view.buffer.active.getLine(0), { cols: 20, cellWidth: 8, theme });
    const boxes = [...element.children] as HTMLElement[];
    expect(boxes.map((box) => box.textContent).join("")).toBe("red bold");
    expect(boxes[0]!.style.width).toBe("8px");
    expect(boxes[0]!.style.color).toBe("rgb(255, 0, 0)");
    expect(boxes[4]!.style.fontWeight).toBe("bold");
  });

  it("gives a wide character the width of two cells", async () => {
    const view = new Terminal({ cols: 20, rows: 3, allowProposedApi: true });
    await write(view, "日本");
    const element = document.createElement("div");
    drawEdgeRow(element, view.buffer.active.getLine(0), { cols: 20, cellWidth: 8, theme });
    expect([...element.children].map((box) => (box as HTMLElement).style.width)).toEqual(["16px", "16px"]);
  });

  it("swaps the colours of inverse cells", async () => {
    const view = new Terminal({ cols: 20, rows: 3, allowProposedApi: true });
    await write(view, "\x1b[7mx\x1b[0m");
    const element = document.createElement("div");
    drawEdgeRow(element, view.buffer.active.getLine(0), { cols: 20, cellWidth: 8, theme });
    const box = element.firstElementChild as HTMLElement;
    expect(box.style.color).toBe("rgb(0, 0, 0)");
    expect(box.style.backgroundColor).toBe("rgb(221, 221, 221)");
  });
});

describe("paletteColour", () => {
  it("takes the 16 colours from the theme, and the cube and the greys as xterm defines them", () => {
    expect(paletteColour(1, theme)).toBe("#ff0000");
    expect(paletteColour(12, theme)).toBe("#8888ff");
    expect(paletteColour(16, theme)).toBe("#000000");
    expect(paletteColour(196, theme)).toBe("#ff0000");
    expect(paletteColour(232, theme)).toBe("#080808");
    expect(paletteColour(255, theme)).toBe("#eeeeee");
  });
});
