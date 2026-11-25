import { fireEvent } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { attachFontZoom } from "./attachFontZoom";

describe("attachFontZoom", () => {
  let container: HTMLDivElement;
  let screen: HTMLDivElement;
  let size: number;
  let detach: () => void;

  beforeEach(() => {
    container = document.createElement("div");
    screen = document.createElement("div");
    container.append(screen);
    document.body.append(container);
    size = 14;
    detach = attachFontZoom(container, { fontSize: () => size, setFontSize: (next) => { size = next; } });
  });

  afterEach(() => {
    detach();
    container.remove();
    vi.useRealTimers();
  });

  it("makes the text a pixel larger for each notch of the wheel turned up with Ctrl, and keeps it from the screen", () => {
    let reachedScreen = false;
    screen.addEventListener("wheel", () => { reachedScreen = true; });

    const turned = fireEvent.wheel(screen, { deltaY: -100, ctrlKey: true });
    expect(size).toBe(15);
    expect(turned).toBe(false);
    expect(reachedScreen).toBe(false);

    fireEvent.wheel(screen, { deltaY: 100, ctrlKey: true });
    fireEvent.wheel(screen, { deltaY: 100, ctrlKey: true });
    expect(size).toBe(13);
  });

  it("adds up a trackpad's small turns, and leaves the wheel without Ctrl to scroll", () => {
    for (let turn = 0; turn < 4; turn += 1) fireEvent.wheel(screen, { deltaY: -10, ctrlKey: true });
    expect(size).toBe(15);

    let reachedScreen = false;
    screen.addEventListener("wheel", () => { reachedScreen = true; });
    fireEvent.wheel(screen, { deltaY: -100 });
    expect(size).toBe(15);
    expect(reachedScreen).toBe(true);
  });

  it("grows the text with the gap between two fingers, and does nothing for one", () => {
    fireEvent.touchStart(screen, { touches: [{ clientX: 100, clientY: 100 }] });
    fireEvent.touchMove(screen, { touches: [{ clientX: 100, clientY: 300 }] });
    expect(size).toBe(14);

    fireEvent.touchStart(screen, { touches: [{ clientX: 100, clientY: 100 }, { clientX: 200, clientY: 100 }] });
    fireEvent.touchMove(screen, { touches: [{ clientX: 50, clientY: 100 }, { clientX: 250, clientY: 100 }] });
    expect(size).toBe(28);
    fireEvent.touchMove(screen, { touches: [{ clientX: 125, clientY: 100 }, { clientX: 175, clientY: 100 }] });
    expect(size).toBe(8);

    fireEvent.touchEnd(screen, { touches: [{ clientX: 125, clientY: 100 }] });
    fireEvent.touchMove(screen, { touches: [{ clientX: 0, clientY: 100 }, { clientX: 300, clientY: 100 }] });
    expect(size).toBe(8);
  });

  it("leaves the size alone when Ctrl goes down while a scroll is still coming, and zooms again after a pause", () => {
    vi.useFakeTimers({ toFake: ["performance"] });
    fireEvent.wheel(screen, { deltaY: -40 });
    // The trackpad coasts: a turn every frame, now with Ctrl held.
    for (let frame = 0; frame < 30; frame += 1) {
      vi.advanceTimersByTime(16);
      const turned = fireEvent.wheel(screen, { deltaY: -40, ctrlKey: true });
      // The page must not zoom either.
      expect(turned).toBe(false);
    }
    expect(size).toBe(14);

    vi.advanceTimersByTime(300);
    fireEvent.wheel(screen, { deltaY: -100, ctrlKey: true });
    expect(size).toBe(15);
  });
});
