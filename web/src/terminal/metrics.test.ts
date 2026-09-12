import { describe, expect, it, vi } from "vitest";
import { cellHeight, measureCells, observeTerminalSize, syncTerminalInputPosition } from "./metrics";

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

function withResizeHarness(run: (harness: {
  notify: () => void;
  flush: () => void;
  pending: () => number;
  observed: Set<Element>;
  disconnect: () => void;
  cancel: (id: number) => void;
}) => void) {
  const previousObserver = Object.getOwnPropertyDescriptor(globalThis, "ResizeObserver");
  const observed = new Set<Element>();
  const frames = new Map<number, FrameRequestCallback>();
  let nextFrame = 0;
  let notify: () => void = () => { throw new Error("ResizeObserver has not been created"); };
  const disconnect = vi.fn(() => observed.clear());
  const cancel = vi.fn((id: number) => { frames.delete(id); });
  class TestResizeObserver implements ResizeObserver {
    constructor(callback: ResizeObserverCallback) {
      notify = () => callback([...observed].map((target) => {
        const contentRect = target.getBoundingClientRect();
        const size = { inlineSize: contentRect.width, blockSize: contentRect.height };
        return { target, contentRect, contentBoxSize: [size], borderBoxSize: [size], devicePixelContentBoxSize: [size] };
      }), this);
    }
    observe(target: Element) { observed.add(target); }
    unobserve(target: Element) { observed.delete(target); }
    disconnect = disconnect;
  }
  Object.defineProperty(globalThis, "ResizeObserver", { configurable: true, value: TestResizeObserver });
  const requestFrame = vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
    frames.set(++nextFrame, callback);
    return nextFrame;
  });
  const cancelFrame = vi.spyOn(window, "cancelAnimationFrame").mockImplementation(cancel);
  try {
    run({
      notify: () => notify(),
      flush: () => {
        const queued = [...frames.values()];
        frames.clear();
        queued.forEach((callback) => callback(0));
      },
      pending: () => frames.size,
      observed,
      disconnect,
      cancel,
    });
  } finally {
    requestFrame.mockRestore();
    cancelFrame.mockRestore();
    if (previousObserver === undefined) Reflect.deleteProperty(globalThis, "ResizeObserver");
    else Object.defineProperty(globalThis, "ResizeObserver", previousObserver);
  }
}

describe("observeTerminalSize", () => {
  it("coalesces host and renderer geometry changes into one fit", () => {
    withResizeHarness((harness) => {
      const host = document.createElement("div");
      let hostRect = rect(800, 500);
      let screenRect = rect(780, 480);
      host.getBoundingClientRect = () => hostRect;
      const view = terminal({ rows: 30 });
      view.screen.getBoundingClientRect = () => screenRect;
      const fit = vi.fn();
      const stop = observeTerminalSize(view, host, fit);
      try {
        expect(harness.observed).toEqual(new Set([host, view.screen]));
        hostRect = rect(800, 400);
        harness.notify();
        screenRect = rect(780, 500);
        harness.notify();
        expect(fit).not.toHaveBeenCalled();
        expect(harness.pending()).toBe(1);
        harness.flush();
        expect(fit).toHaveBeenCalledTimes(1);
      } finally { stop(); }
    });
  });

  it("ignores its own fit geometry while following later external changes", () => {
    withResizeHarness((harness) => {
      const host = document.createElement("div");
      let hostRect = rect(800, 46);
      let screenHeight = 15;
      host.getBoundingClientRect = () => hostRect;
      const view = terminal({ rows: 1 });
      view.screen.getBoundingClientRect = () => rect(780, screenHeight);
      // At fractional DPI, DOM rows can round to 15px for one row and 31px
      // for two. Refitting the fit's own resize would oscillate indefinitely.
      const fit = vi.fn(() => { screenHeight = screenHeight === 15 ? 31 : 15; });
      const stop = observeTerminalSize(view, host, fit);
      try {
        screenHeight = 31;
        harness.notify();
        harness.flush();
        expect(fit).toHaveBeenCalledTimes(1);
        expect(screenHeight).toBe(15);
        harness.notify();
        expect(harness.pending()).toBe(0);
        harness.flush();
        expect(fit).toHaveBeenCalledTimes(1);

        hostRect = rect(800, 62);
        harness.notify();
        harness.flush();
        expect(fit).toHaveBeenCalledTimes(2);
        harness.notify();
        expect(harness.pending()).toBe(0);

        screenHeight = 16;
        harness.notify();
        harness.flush();
        expect(fit).toHaveBeenCalledTimes(3);
      } finally { stop(); }
    });
  });

  it("skips a queued fit when geometry has already returned to its prior size", () => {
    withResizeHarness((harness) => {
      const host = document.createElement("div");
      let hostHeight = 500;
      host.getBoundingClientRect = () => rect(800, hostHeight);
      const view = terminal({ rows: 30, screen: rect(780, 480) });
      const fit = vi.fn();
      const stop = observeTerminalSize(view, host, fit);
      try {
        hostHeight = 400;
        harness.notify();
        expect(harness.pending()).toBe(1);
        hostHeight = 500;
        harness.flush();
        expect(fit).not.toHaveBeenCalled();
      } finally { stop(); }
    });
  });

  it("disconnects and cancels a pending fit when disposed", () => {
    withResizeHarness((harness) => {
      const host = document.createElement("div");
      let hostHeight = 500;
      host.getBoundingClientRect = () => rect(800, hostHeight);
      const view = terminal({ rows: 30, screen: rect(780, 480) });
      const fit = vi.fn();
      const stop = observeTerminalSize(view, host, fit);
      hostHeight = 400;
      harness.notify();
      expect(harness.pending()).toBe(1);
      stop();
      expect(harness.disconnect).toHaveBeenCalledOnce();
      expect(harness.cancel).toHaveBeenCalledOnce();
      expect(harness.observed.size).toBe(0);
      expect(harness.pending()).toBe(0);
      harness.flush();
      expect(fit).not.toHaveBeenCalled();
    });
  });
});

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
