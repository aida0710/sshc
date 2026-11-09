import { act, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { mobileViewportQuery } from "../ui/useMediaQuery";
import { useBackGestureMotion } from "./backGestureMotion";

function Page({ backMovesPage }: { backMovesPage: () => boolean }) {
  useBackGestureMotion(backMovesPage);
  return <div className="sshc-app" data-testid="app" />;
}

function swipe(phase: string, progress: number, edge = "left") {
  act(() => {
    window.dispatchEvent(new CustomEvent("sshc-android-back-gesture", { detail: { phase, progress, edge } }));
  });
}

// jsdom runs no transitions; the page tells when its transform has landed.
function transformLanded(app: HTMLElement) {
  const landed = new Event("transitionend");
  Object.defineProperty(landed, "propertyName", { value: "transform" });
  act(() => {
    app.dispatchEvent(landed);
  });
}

const root = document.documentElement;

function onPhone() {
  vi.stubGlobal("matchMedia", vi.fn((query: string) => ({ matches: query === mobileViewportQuery, media: query, addEventListener: vi.fn(), removeEventListener: vi.fn() })));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("useBackGestureMotion", () => {
  it("draws the app itself smaller as the finger moves, and grows it back when the gesture is taken back", () => {
    onPhone();
    const { getByTestId } = render(<Page backMovesPage={() => true} />);
    const app = getByTestId("app");
    swipe("start", 0, "right");
    expect(root.dataset.backGesture).toBe("");
    swipe("progress", 0.5, "right");
    expect(app.style.transform).toBe("translateX(-8px) scale(0.95)");
    expect(app.style.borderRadius).toBe("14px");
    // Nothing is written on <html>, whose variables every element would have to take again.
    expect(root.style.length).toBe(0);
    swipe("cancel", 0);
    expect(app.style.transform).toBe("translateX(0px) scale(1)");
    transformLanded(app);
    expect(root.dataset.backGesture).toBeUndefined();
    expect(app.style.transform).toBe("");
    expect(app.style.borderRadius).toBe("");
  });

  it("does not move a page whose Back only closes something", () => {
    onPhone();
    const { getByTestId } = render(<Page backMovesPage={() => false} />);
    swipe("start", 0);
    swipe("progress", 0.6);
    expect(root.dataset.backGesture).toBeUndefined();
    expect(getByTestId("app").style.transform).toBe("");
  });

  it("does not move the page with reduced motion", () => {
    onPhone();
    root.dataset.motion = "reduce";
    const { getByTestId } = render(<Page backMovesPage={() => true} />);
    swipe("start", 0);
    swipe("progress", 0.4);
    expect(getByTestId("app").style.transform).toBe("");
    delete root.dataset.motion;
  });
});
