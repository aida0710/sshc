import { describe, expect, it, vi } from "vitest";
import { attachSelectionOverlay, overlayClass, selectionHeldIn } from "./selectionOverlay";

function harness(lines: string[]) {
  const container = document.createElement("div");
  document.body.appendChild(container);

  const element = document.createElement("div");
  element.className = "xterm";
  const screen = document.createElement("div");
  screen.className = "xterm-screen";
  const rows = document.createElement("div");
  rows.className = "xterm-rows";
  screen.appendChild(rows);
  element.appendChild(screen);
  container.appendChild(element);

  let repaint = () => {};
  const view = {
    element,
    rows: lines.length,
    cols: 40,
    buffer: {
      active: {
        viewportY: 0,
        length: lines.length,
        getLine: (index: number) => {
          const line = lines[index];
          if (line === undefined) return undefined;
          return { translateToString: (trim?: boolean) => (trim ? line.trimEnd() : line) };
        },
      },
    },
    focus: vi.fn(),
    blur: vi.fn(),
    onRender: (handler: () => void) => {
      repaint = handler;
      return { dispose: vi.fn() };
    },
  };

  const detach = attachSelectionOverlay(container, view);
  const overlay = container.querySelector(`.${overlayClass}`) as HTMLElement;
  return { container, overlay, view, screen, detach, repaint: () => repaint() };
}

describe("attachSelectionOverlay", () => {
  it("hangs the text outside the element that cannot be selected", () => {
    const { container, overlay, detach } = harness(["one", "two"]);
    expect(overlay).not.toBeNull();
    expect(overlay.closest(".xterm")).toBeNull();
    expect(overlay.parentElement).toBe(container);
    detach();
  });

  it("carries the visible rows", () => {
    const { overlay, detach } = harness(["first line", "second line"]);
    expect(overlay.textContent).toBe("first line\nsecond line");
    detach();
  });

  it("freezes while a selection is held in it", () => {
    const lines = ["before"];
    const { overlay, detach, repaint } = harness(lines);
    const range = document.createRange();
    range.selectNodeContents(overlay);
    const selection = document.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    expect(selectionHeldIn(overlay)).toBe(true);

    lines[0] = "after";
    repaint();
    expect(overlay.textContent).toBe("before");

    selection?.removeAllRanges();
    repaint();
    expect(overlay.textContent).toBe("after");
    detach();
  });

  it("lets go of the selection when the terminal is laid out again", () => {
    const lines = ["before"];
    const { overlay, detach, repaint, screen } = harness(lines);
    const range = document.createRange();
    range.selectNodeContents(overlay);
    const selection = document.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    expect(selectionHeldIn(overlay)).toBe(true);

    lines[0] = "after";
    screen.getBoundingClientRect = () =>
      ({ left: 0, top: 0, width: 200, height: 100 }) as DOMRect;
    repaint();

    expect(selectionHeldIn(overlay)).toBe(false);
    expect(overlay.textContent).toBe("after");
    detach();
  });

  it("leaves a selection it does not hold alone when the terminal is laid out again", () => {
    const { detach, repaint, screen } = harness(["before"]);
    const elsewhere = document.createElement("div");
    elsewhere.textContent = "someone else's text";
    document.body.appendChild(elsewhere);
    const range = document.createRange();
    range.selectNodeContents(elsewhere);
    const selection = document.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);

    screen.getBoundingClientRect = () => ({ left: 0, top: 0, width: 200, height: 100 }) as DOMRect;
    repaint();

    expect(document.getSelection()?.toString()).toBe("someone else's text");
    selection?.removeAllRanges();
    elsewhere.remove();
    detach();
  });

  it("swallows the mouse event Chromium sends after a finger", () => {
    const { overlay, detach } = harness(["one"]);
    const event = new MouseEvent("mousedown", { bubbles: true, cancelable: true });
    overlay.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
    detach();
  });

  it("hands the terminal the focus after a tap", () => {
    const { overlay, view, detach } = harness(["one"]);
    const touch = new Event("touchstart", { bubbles: true });
    Object.defineProperty(touch, "touches", { value: [{ clientY: 10 }] });
    overlay.dispatchEvent(touch);
    overlay.dispatchEvent(new Event("touchend", { bubbles: true }));
    expect(view.focus).toHaveBeenCalled();
    detach();
  });

  it("takes the node away again", () => {
    const { container, detach } = harness(["one"]);
    detach();
    expect(container.querySelector(`.${overlayClass}`)).toBeNull();
  });
});

describe("selectionHeldIn", () => {
  it("is false for a collapsed caret", () => {
    const node = document.createElement("div");
    node.textContent = "text";
    document.body.appendChild(node);
    const range = document.createRange();
    range.setStart(node, 0);
    range.collapse(true);
    const selection = document.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    expect(selectionHeldIn(node)).toBe(false);
    selection?.removeAllRanges();
    node.remove();
  });

  it("is false when the selection lives elsewhere", () => {
    const mine = document.createElement("div");
    const theirs = document.createElement("div");
    theirs.textContent = "not mine";
    document.body.append(mine, theirs);
    const range = document.createRange();
    range.selectNodeContents(theirs);
    const selection = document.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    expect(selectionHeldIn(mine)).toBe(false);
    selection?.removeAllRanges();
    mine.remove();
    theirs.remove();
  });
});
