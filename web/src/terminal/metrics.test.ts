import { describe, expect, it } from "vitest";
import { cellHeight, measureCells, syncTerminalInputPosition } from "./metrics";

function terminal(options: { screen?: DOMRect; rows: number; letterSpacing?: string }) {
  const element = document.createElement("div");
  element.className = "xterm";
  const screen = document.createElement("div");
  screen.className = "xterm-screen";
  const glyphs = document.createElement("div");
  glyphs.className = "xterm-rows";
  if (options.letterSpacing !== undefined) {
    glyphs.style.letterSpacing = options.letterSpacing;
    glyphs.style.fontFamily = "Menlo";
    glyphs.style.fontSize = "13px";
  }
  screen.appendChild(glyphs);
  element.appendChild(screen);
  document.body.appendChild(element);
  if (options.screen !== undefined) {
    const box = options.screen;
    screen.getBoundingClientRect = () => box;
  }
  return { element, rows: options.rows, screen };
}

function rect(width: number, height: number, left = 0, top = 0): DOMRect {
  return { width, height, left, top, right: left + width, bottom: top + height, x: left, y: top, toJSON: () => ({}) };
}

describe("measureCells", () => {
  it("divides the drawn surface by the number of rows", () => {
    const view = terminal({ screen: rect(800, 480), rows: 30 });
    expect(measureCells(view)?.cellHeight).toBe(16);
  });

  it("keeps the fraction", () => {
    const view = terminal({ screen: rect(800, 521), rows: 30 });
    expect(measureCells(view)?.cellHeight).toBeCloseTo(17.3667, 3);
  });

  it("copies the calibration the terminal resolved, not a guess", () => {
    const view = terminal({ screen: rect(800, 480), rows: 30, letterSpacing: "0.55px" });
    expect(measureCells(view)?.font.letterSpacing).toBe("0.55px");
    expect(measureCells(view)?.font.family).toBe("Menlo");
  });

  it("measures WebGL terminals after the DOM rows have been replaced", () => {
    const view = {
      ...terminal({ screen: rect(832, 146), rows: 8 }),
      options: { fontFamily: "Menlo", fontSize: 15, fontWeight: 400, letterSpacing: 0.5 },
    };
    view.element.querySelector(".xterm-rows")?.remove();
    expect(measureCells(view)).toMatchObject({
      rect: { width: 832, height: 146 },
      cellHeight: 18.25,
      font: { family: "Menlo", size: "15px", weight: "400", letterSpacing: "0.5px" },
    });
    view.screen.getBoundingClientRect = () => rect(378, 567);
    view.rows = 31;
    expect(measureCells(view)?.rect).toMatchObject({ width: 378, height: 567 });
  });

  it("says nothing rather than zero while the surface is not up", () => {
    expect(measureCells({ element: undefined, rows: 30 })).toBeNull();
    expect(measureCells(terminal({ screen: rect(0, 0), rows: 30 }))).toBeNull();
    expect(measureCells(terminal({ screen: rect(800, 480), rows: 0 }))).toBeNull();
  });
});

describe("syncTerminalInputPosition", () => {
  it("moves the IME input with the resized cursor even without a cursor-move event", () => {
    const textarea = document.createElement("textarea");
    textarea.style.top = "540px";
    textarea.style.left = "390px";
    textarea.value = "ongoing composition";
    const view = {
      ...terminal({ screen: rect(800, 18), rows: 1 }),
      cols: 100,
      textarea,
      buffer: { active: { cursorX: 5, cursorY: 0 } },
    };
    syncTerminalInputPosition(view);
    expect(textarea.style.top).toBe("0px");
    expect(textarea.style.left).toBe("40px");
    expect(textarea.value).toBe("ongoing composition");
  });
});

describe("cellHeight", () => {
  it("falls back to the box until the surface is up", () => {
    const box = document.createElement("div");
    box.getBoundingClientRect = () => rect(800, 240);
    expect(cellHeight({ element: undefined, rows: 24 }, box)).toBe(10);
  });

  it("prefers the surface once it exists", () => {
    const box = document.createElement("div");
    box.getBoundingClientRect = () => rect(800, 999);
    expect(cellHeight(terminal({ screen: rect(800, 240), rows: 24 }), box)).toBe(10);
  });

  it("refuses to divide by no rows", () => {
    const box = document.createElement("div");
    box.getBoundingClientRect = () => rect(800, 240);
    expect(cellHeight({ element: undefined, rows: 0 }, box)).toBe(0);
  });
});

describe("xterm の DOM を知る場所", () => {
  it("is metrics.ts and nowhere else", async () => {
    const { readdir, readFile } = await import("node:fs/promises");
    const { dirname, join } = await import("node:path");
    const { fileURLToPath } = await import("node:url");
    const sourceRoot = join(dirname(fileURLToPath(import.meta.url)), "..");

    async function sources(directory: string): Promise<string[]> {
      const entries = await readdir(directory, { withFileTypes: true });
      const found: string[] = [];
      for (const entry of entries) {
        const path = join(directory, entry.name);
        if (entry.isDirectory()) found.push(...(await sources(path)));
        else if (/\.tsx?$/.test(entry.name) && !entry.name.includes(".test.")) found.push(path);
      }
      return found;
    }

    const scrapes = /(?:querySelector|querySelectorAll|closest|matches|getElementsByClassName)\s*(?:<[^>]*>)?\s*\(\s*["'`][^"'`]*xterm/;
    const elsewhere: string[] = [];
    for (const path of await sources(sourceRoot)) {
      if (path.endsWith(join("terminal", "metrics.ts"))) continue;
      if (scrapes.test(await readFile(path, "utf8"))) elsewhere.push(path);
    }
    expect(elsewhere).toEqual([]);
  });

  it("is support/terminal.ts and nowhere else in the e2e suite", async () => {
    const { readdir, readFile } = await import("node:fs/promises");
    const { dirname, join } = await import("node:path");
    const { fileURLToPath } = await import("node:url");
    const e2eRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "e2e");

    async function specs(directory: string): Promise<string[]> {
      const entries = await readdir(directory, { withFileTypes: true });
      const found: string[] = [];
      for (const entry of entries) {
        const path = join(directory, entry.name);
        if (entry.isDirectory()) found.push(...(await specs(path)));
        else if (/\.ts$/.test(entry.name)) found.push(path);
      }
      return found;
    }

    const reaches = /(?:locator|querySelector|querySelectorAll|closest)\s*\(\s*["'`][^"'`]*\.xterm/;
    const elsewhere: string[] = [];
    for (const path of await specs(e2eRoot)) {
      if (path.endsWith(join("support", "terminal.ts"))) continue;
      if (reaches.test(await readFile(path, "utf8"))) elsewhere.push(path);
    }
    expect(elsewhere).toEqual([]);
  });
});
