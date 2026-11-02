import { Terminal } from "@xterm/xterm";
import { describe, expect, it } from "vitest";
import { createScrollbackOffset } from "./scrollbackOffset";

function write(view: Terminal, data: string): Promise<void> {
  return new Promise((resolve) => view.write(data, resolve));
}

// scrolledBack is a terminal of 4 rows with 10 lines of output, scrolled back to the top, whose rows are drawn 80 px
// wide and 20 px a row at the top left of its host.
async function scrolledBack() {
  const buffer = new Terminal({ cols: 10, rows: 4, allowProposedApi: true });
  await write(buffer, Array.from({ length: 10 }, (_, line) => `line ${line}`).join("\r\n"));
  buffer.scrollToTop();
  const host = document.createElement("div");
  const element = document.createElement("div");
  const screen = document.createElement("div");
  screen.className = "xterm-screen";
  screen.getBoundingClientRect = () => ({ left: 0, top: 0, right: 80, bottom: 80, width: 80, height: 80 }) as DOMRect;
  element.appendChild(screen);
  host.appendChild(element);
  host.getBoundingClientRect = () => ({ left: 0, top: 0, right: 80, bottom: 90, width: 80, height: 90 }) as DOMRect;
  const view = { element, rows: buffer.rows, cols: buffer.cols, buffer: buffer.buffer, options: { theme: { foreground: "#dddddd" } } };
  const offset = createScrollbackOffset(host, view);
  const edge = host.querySelector<HTMLElement>("[data-scrollback-edge-row]")!;
  return { buffer, host, offset, edge };
}

describe("createScrollbackOffset", () => {
  it("says how far the scrollback can move from the rows shown", async () => {
    const { buffer, offset } = await scrolledBack();
    expect(offset.room()).toEqual({ above: 0, below: 6 });
    buffer.scrollLines(2);
    expect(offset.room()).toEqual({ above: 2, below: 4 });
  });

  it("moves the drawn terminal up by part of a row and draws the next row under it", async () => {
    const { host, offset, edge } = await scrolledBack();
    offset.show(0.25);
    expect(host.style.transform).toBe("translateY(-5px)");
    expect(edge.style.display).toBe("block");
    expect(edge.style.top).toBe("80px");
    expect(edge.textContent).toBe("line 4");
  });

  it("puts everything back once the rows are on a row", async () => {
    const { host, offset, edge } = await scrolledBack();
    offset.show(0.5);
    offset.show(0);
    expect(host.style.transform).toBe("");
    expect(edge.style.display).toBe("none");
  });

  it("does not move a program's alternate screen, which has no scrollback", async () => {
    const { buffer, offset } = await scrolledBack();
    await write(buffer, "\x1b[?1049h");
    expect(offset.room()).toEqual({ above: 0, below: 0 });
  });
});
