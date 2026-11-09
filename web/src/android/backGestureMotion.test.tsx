import { act, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { mobileViewportQuery } from "../ui/useMediaQuery";
import { useBackGestureMotion } from "./backGestureMotion";

function Page({ backMovesPage }: { backMovesPage: () => boolean }) {
  useBackGestureMotion(backMovesPage);
  return null;
}

function swipe(phase: string, progress: number, edge = "left") {
  act(() => {
    window.dispatchEvent(new CustomEvent("sshc-android-back-gesture", { detail: { phase, progress, edge } }));
  });
}

const root = document.documentElement;

function onPhone() {
  vi.stubGlobal("matchMedia", vi.fn((query: string) => ({ matches: query === mobileViewportQuery, media: query, addEventListener: vi.fn(), removeEventListener: vi.fn() })));
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("useBackGestureMotion", () => {
  it("follows the finger while Back would go to another page, and grows back when the gesture is taken back", () => {
    onPhone();
    vi.useFakeTimers();
    render(<Page backMovesPage={() => true} />);
    swipe("start", 0, "right");
    expect(root.dataset.backGesture).toBe("");
    swipe("progress", 0.5, "right");
    expect(root.style.getPropertyValue("--sshc-back-progress")).toBe("0.5");
    expect(root.style.getPropertyValue("--sshc-back-edge")).toBe("-1");
    swipe("cancel", 0);
    expect(root.dataset.backGesture).toBe("settling");
    act(() => {
      vi.runAllTimers();
    });
    expect(root.dataset.backGesture).toBeUndefined();
    expect(root.style.getPropertyValue("--sshc-back-progress")).toBe("");
  });

  it("does not move a page whose Back only closes something", () => {
    onPhone();
    render(<Page backMovesPage={() => false} />);
    swipe("start", 0);
    swipe("progress", 0.6);
    expect(root.dataset.backGesture).toBeUndefined();
    expect(root.style.getPropertyValue("--sshc-back-progress")).toBe("");
  });

  it("does not move the page with reduced motion", () => {
    onPhone();
    root.dataset.motion = "reduce";
    render(<Page backMovesPage={() => true} />);
    swipe("start", 0);
    expect(root.dataset.backGesture).toBeUndefined();
    delete root.dataset.motion;
  });
});
