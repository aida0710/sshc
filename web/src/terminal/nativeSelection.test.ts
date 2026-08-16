import { describe, expect, it, vi } from "vitest";
import {
  attachNativeSelection,
  hasSelection,
  isTap,
  nativeSelectionClass,
  prefersNativeSelection,
  tapMillis,
  tapSlopPixels,
} from "./nativeSelection";

describe("prefersNativeSelection", () => {
  it("asks for the screens that have no hover and a coarse pointer", () => {
    const match = vi.fn().mockReturnValue({ matches: true });
    expect(prefersNativeSelection(match)).toBe(true);
    expect(match).toHaveBeenCalledWith("(hover: none) and (pointer: coarse)");
  });

  it("stays out of the way where a mouse exists", () => {
    expect(prefersNativeSelection(() => ({ matches: false }))).toBe(false);
  });
});

describe("isTap", () => {
  // **長押しは選択であって、焦点の移動ではない。** ここを取り違えると、
  // OS が掴んだ範囲は焦点が動いた瞬間に外れる。
  it("calls a short, still touch a tap", () => {
    expect(isTap(120, 3)).toBe(true);
    expect(isTap(tapMillis, tapSlopPixels)).toBe(true);
  });

  it("refuses a touch that was held", () => {
    expect(isTap(tapMillis + 1, 0)).toBe(false);
  });

  it("refuses a touch that travelled", () => {
    expect(isTap(50, tapSlopPixels + 1)).toBe(false);
  });
});

describe("hasSelection", () => {
  const selection = (collapsed: boolean, text: string) =>
    ({ isCollapsed: collapsed, toString: () => text }) as Selection;

  it("sees a real range", () => {
    expect(hasSelection(selection(false, "ls -la"))).toBe(true);
  });

  // 折り畳まれたキャレットは選択ではない。ここを取り違えると、一度触れた
  // だけで指のスクロールが永久に止まる。
  it("does not mistake a caret for a range", () => {
    expect(hasSelection(selection(true, ""))).toBe(false);
    expect(hasSelection(selection(false, ""))).toBe(false);
    expect(hasSelection(null)).toBe(false);
  });
});

describe("attachNativeSelection", () => {
  function harness() {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const focus = vi.fn();
    let clock = 0;
    const detach = attachNativeSelection({ container, focus, now: () => clock });
    return { container, focus, detach, tick: (ms: number) => (clock += ms) };
  }

  function touch(type: string, x: number, y: number): TouchEvent {
    const event = new Event(type, { bubbles: true }) as unknown as {
      touches: unknown;
      changedTouches: unknown;
    };
    const finger = [{ clientX: x, clientY: y }];
    event.touches = finger;
    event.changedTouches = finger;
    return event as unknown as TouchEvent;
  }

  // **xterm の mousedown を見せない。** あれは無条件に preventDefault を
  // 呼び、長押しが生む mousedown もそこで潰れる。
  it("stops mousedown before anything below can see it", () => {
    const { container, detach } = harness();
    const below = vi.fn();
    container.addEventListener("mousedown", below);
    container.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    expect(below).not.toHaveBeenCalled();
    detach();
  });

  it("gives the focus back on a tap", () => {
    const { container, focus, detach, tick } = harness();
    container.dispatchEvent(touch("touchstart", 100, 100));
    tick(80);
    container.dispatchEvent(touch("touchend", 102, 101));
    expect(focus).toHaveBeenCalledTimes(1);
    detach();
  });

  it("leaves a long press alone", () => {
    const { container, focus, detach, tick } = harness();
    container.dispatchEvent(touch("touchstart", 100, 100));
    tick(tapMillis + 200);
    container.dispatchEvent(touch("touchend", 100, 100));
    expect(focus).not.toHaveBeenCalled();
    detach();
  });

  it("marks and unmarks the element the stylesheet looks for", () => {
    const { container, detach } = harness();
    expect(container.classList.contains(nativeSelectionClass)).toBe(true);
    detach();
    expect(container.classList.contains(nativeSelectionClass)).toBe(false);
  });

  it("stops swallowing once detached", () => {
    const { container, detach } = harness();
    detach();
    const below = vi.fn();
    container.addEventListener("mousedown", below);
    container.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    expect(below).toHaveBeenCalledTimes(1);
  });
});
