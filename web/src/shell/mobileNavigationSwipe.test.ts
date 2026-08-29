import { describe, expect, it } from "vitest";
import {
  shouldOpenMobileNavigation,
  shouldStartMobileNavigationSwipe,
} from "./mobileNavigationSwipe";

describe("mobile navigation swipe", () => {
  it("opens after a horizontal swipe from the left edge on a narrow screen", () => {
    const start = { clientX: 12, clientY: 240 };
    expect(shouldStartMobileNavigationSwipe(start, 390)).toBe(true);
    expect(shouldOpenMobileNavigation(start, { clientX: 100, clientY: 252 })).toBe(true);
  });

  it("does not start away from the edge, on desktop, or with multiple fingers", () => {
    expect(shouldStartMobileNavigationSwipe({ clientX: 40, clientY: 100 }, 390)).toBe(false);
    expect(shouldStartMobileNavigationSwipe({ clientX: 12, clientY: 100 }, 1280)).toBe(false);
    expect(shouldStartMobileNavigationSwipe({ clientX: 12, clientY: 100 }, 390, 2)).toBe(false);
  });

  it("does not mistake a short or vertical gesture for opening navigation", () => {
    const start = { clientX: 8, clientY: 100 };
    expect(shouldOpenMobileNavigation(start, { clientX: 60, clientY: 104 })).toBe(false);
    expect(shouldOpenMobileNavigation(start, { clientX: 92, clientY: 180 })).toBe(false);
  });
});
