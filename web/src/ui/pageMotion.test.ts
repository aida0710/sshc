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

afterEach(() => {
  vi.unstubAllGlobals();
  Reflect.deleteProperty(document, "startViewTransition");
  delete document.documentElement.dataset.motion;
  delete document.documentElement.dataset.backGesture;
});

describe("presentLocationChange", () => {
  it("fades to another page on a phone, also when Android's Back replaces the entry with another section", () => {
    const startViewTransition = onScreen({ phone: true });
    const change = vi.fn();
    presentLocationChange(change, { from: home, to: connections, replaced: false });
    presentLocationChange(change, { from: connections, to: home, replaced: true });
    expect(startViewTransition).toHaveBeenCalledTimes(2);
    expect(change).toHaveBeenCalledTimes(2);
  });

  it("does not fade when a page settles its own address in its section", () => {
    const startViewTransition = onScreen({ phone: true });
    const change = vi.fn();
    presentLocationChange(change, { from: { pathname: "/connections", search: "" }, to: connections, replaced: true });
    expect(startViewTransition).not.toHaveBeenCalled();
    expect(change).toHaveBeenCalledOnce();
  });

  it("changes the page at once on the same page, with reduced motion, or on a wide screen", () => {
    const startViewTransition = onScreen({ phone: true });
    const change = vi.fn();
    presentLocationChange(change, { from: home, to: home, replaced: false });
    document.documentElement.dataset.motion = "reduce";
    presentLocationChange(change, { from: home, to: connections, replaced: false });
    delete document.documentElement.dataset.motion;
    const onWideScreen = onScreen({ phone: false });
    presentLocationChange(change, { from: home, to: connections, replaced: false });
    expect(startViewTransition).not.toHaveBeenCalled();
    expect(onWideScreen).not.toHaveBeenCalled();
    expect(change).toHaveBeenCalledTimes(3);
  });

  it("draws the page whole after a back gesture left it small", () => {
    onScreen({ phone: true });
    document.documentElement.dataset.backGesture = "";
    presentLocationChange(vi.fn(), { from: connections, to: home, replaced: true });
    expect(document.documentElement.dataset.backGesture).toBeUndefined();
  });
});
