import { afterEach, describe, expect, it, vi } from "vitest";
import { isTerminalShown, watchTerminalShown } from "./terminalVisibility";

let resized: () => void = () => {};
class FakeResizeObserver {
  constructor(callback: () => void) { resized = callback; }
  observe() {}
  disconnect() {}
}

function sized(width: number, height: number): HTMLElement {
  const container = document.createElement("div");
  Object.defineProperty(container, "clientWidth", { configurable: true, get: () => width });
  Object.defineProperty(container, "clientHeight", { configurable: true, get: () => height });
  return container;
}

function setSize(container: HTMLElement, width: number, height: number) {
  Object.defineProperty(container, "clientWidth", { configurable: true, get: () => width });
  Object.defineProperty(container, "clientHeight", { configurable: true, get: () => height });
}

afterEach(() => vi.unstubAllGlobals());

describe("whether this browser shows a terminal", () => {
  it("counts a terminal laid out on a page that is not hidden, whatever has the focus", () => {
    expect(isTerminalShown(sized(800, 400))).toBe(true);
    // A pane out of the split or a terminal kept hidden has no size.
    expect(isTerminalShown(sized(0, 0))).toBe(false);
    const hidden = vi.spyOn(document, "hidden", "get").mockReturnValue(true);
    expect(isTerminalShown(sized(800, 400))).toBe(false);
    hidden.mockRestore();
  });

  it("says when that changes, and only then", () => {
    vi.stubGlobal("ResizeObserver", FakeResizeObserver);
    const container = sized(800, 400);
    const changes: boolean[] = [];
    const stop = watchTerminalShown(container, (shown) => changes.push(shown));

    resized();
    setSize(container, 0, 0);
    resized();
    resized();
    setSize(container, 640, 300);
    const hidden = vi.spyOn(document, "hidden", "get").mockReturnValue(true);
    document.dispatchEvent(new Event("visibilitychange"));
    hidden.mockReturnValue(false);
    document.dispatchEvent(new Event("visibilitychange"));
    stop();
    hidden.mockRestore();

    expect(changes).toEqual([false, true]);
  });
});
