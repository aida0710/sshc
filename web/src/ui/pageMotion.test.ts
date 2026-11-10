import { afterEach, describe, expect, it, vi } from "vitest";
import { presentLocationChange } from "./pageMotion";
import { mobileViewportQuery } from "./useMediaQuery";

const home = { pathname: "/", search: "" };
const connections = { pathname: "/connections/servers", search: "" };

function onScreen({ phone }: { phone: boolean }) {
  vi.stubGlobal("matchMedia", vi.fn((query: string) => ({ matches: phone && query === mobileViewportQuery, media: query, addEventListener: vi.fn(), removeEventListener: vi.fn() })));
  const startViewTransition = vi.fn((update: () => void) => {
    update();
    return { finished: Promise.resolve() };
  });
  Object.defineProperty(document, "startViewTransition", { configurable: true, value: startViewTransition });
  return startViewTransition;
}

// Android の戻るジェスチャーがページを小さく描いたまま、指が離れた（backGesturePage.ts）。
function letGoOfBackGesture() {
  document.documentElement.dataset.backGesture = "";
}

afterEach(() => {
  vi.unstubAllGlobals();
  Reflect.deleteProperty(document, "startViewTransition");
  delete document.documentElement.dataset.motion;
  delete document.documentElement.dataset.backGesture;
});

describe("presentLocationChange", () => {
  it("changes the page at once on a phone for a tab, a link or Back, so that the next tap reaches the page", () => {
    const startViewTransition = onScreen({ phone: true });
    const change = vi.fn();
    presentLocationChange(change, { from: home, to: connections, replaced: false });
    presentLocationChange(change, { from: connections, to: home, replaced: true });
    expect(startViewTransition).not.toHaveBeenCalled();
    expect(change).toHaveBeenCalledTimes(2);
  });

  it("fades from the small page Android's back gesture let go of to the page before, drawn whole", () => {
    const startViewTransition = onScreen({ phone: true });
    letGoOfBackGesture();
    const change = vi.fn();
    presentLocationChange(change, { from: connections, to: home, replaced: true });
    expect(startViewTransition).toHaveBeenCalledOnce();
    expect(change).toHaveBeenCalledOnce();
    expect(document.documentElement.dataset.backGesture).toBeUndefined();
  });

  it("does not fade when a page settles its own address in its section", () => {
    const startViewTransition = onScreen({ phone: true });
    letGoOfBackGesture();
    const change = vi.fn();
    presentLocationChange(change, { from: { pathname: "/connections", search: "" }, to: connections, replaced: true });
    expect(startViewTransition).not.toHaveBeenCalled();
    expect(change).toHaveBeenCalledOnce();
  });

  it("changes the page at once after the back gesture on the same page, with reduced motion, or on a wide screen", () => {
    const startViewTransition = onScreen({ phone: true });
    letGoOfBackGesture();
    const change = vi.fn();
    presentLocationChange(change, { from: home, to: home, replaced: false });
    document.documentElement.dataset.motion = "reduce";
    presentLocationChange(change, { from: connections, to: home, replaced: true });
    delete document.documentElement.dataset.motion;
    const onWideScreen = onScreen({ phone: false });
    presentLocationChange(change, { from: connections, to: home, replaced: true });
    expect(startViewTransition).not.toHaveBeenCalled();
    expect(onWideScreen).not.toHaveBeenCalled();
    expect(change).toHaveBeenCalledTimes(3);
  });
});
