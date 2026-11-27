import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useStuckToTop } from "./useStuckToTop";

let report: IntersectionObserverCallback = () => undefined;

// seen は、要素の上の端（top）と、見えている部分の上の端（visibleTop）と、見える割合で、ブラウザの答えを作る。
function seen({ top, visibleTop, ratio }: { top: number; visibleTop: number; ratio: number }) {
  act(() => {
    report([{
      intersectionRatio: ratio,
      // Chrome は、見える割合がしきい値（1）を下回ると isIntersecting を false にする。
      isIntersecting: ratio >= 1,
      boundingClientRect: { top } as DOMRectReadOnly,
      intersectionRect: { top: visibleTop } as DOMRectReadOnly,
    } as IntersectionObserverEntry], {} as IntersectionObserver);
  });
}

describe("useStuckToTop", () => {
  beforeEach(() => {
    vi.stubGlobal("IntersectionObserver", class {
      constructor(callback: IntersectionObserverCallback) { report = callback; }
      observe() {}
      disconnect() {}
    });
  });
  afterEach(() => vi.unstubAllGlobals());

  function render() {
    const ref = { current: document.createElement("div") };
    return renderHook(() => useStuckToTop(ref));
  }

  it("says stuck once the top pixel goes past the top of the scrolling pane, and not once it is back", () => {
    const { result } = render();
    seen({ top: 9, visibleTop: 9, ratio: 1 });
    expect(result.current).toBe(false);

    seen({ top: -1, visibleTop: 0, ratio: 0.995 });
    expect(result.current).toBe(true);

    seen({ top: 9, visibleTop: 9, ratio: 1 });
    expect(result.current).toBe(false);
  });

  it("does not say stuck when only the bottom is cut by a short window", () => {
    const { result } = render();
    seen({ top: 600, visibleTop: 600, ratio: 0.5 });
    expect(result.current).toBe(false);
  });

  it("does not say stuck while the page is hidden", () => {
    const { result } = render();
    seen({ top: 0, visibleTop: 0, ratio: 0 });
    expect(result.current).toBe(false);
  });
});
