import { afterEach, describe, expect, it, vi } from "vitest";
import {
  clampNavigationWidth,
  defaultNavigationWidth,
  detectNavigationWidth,
  maximumNavigationWidth,
  minimumNavigationWidth,
  navigationWidthStorageKey,
  rememberNavigationWidth,
} from "./navigationLayout";

afterEach(() => {
  vi.restoreAllMocks();
  window.localStorage.clear();
});

describe("navigation width", () => {
  it("uses the default for missing and invalid values", () => {
    expect(detectNavigationWidth()).toBe(defaultNavigationWidth);
    for (const invalid of ["", "wide", "NaN", "Infinity"]) {
      window.localStorage.setItem(navigationWidthStorageKey, invalid);
      expect(detectNavigationWidth()).toBe(defaultNavigationWidth);
    }
  });

  it("rounds and clamps widths to the supported range", () => {
    expect(clampNavigationWidth(260.6)).toBe(261);
    expect(clampNavigationWidth(minimumNavigationWidth - 100)).toBe(minimumNavigationWidth);
    expect(clampNavigationWidth(maximumNavigationWidth + 100)).toBe(maximumNavigationWidth);
    expect(clampNavigationWidth(Number.NaN)).toBe(defaultNavigationWidth);
  });

  it("normalises remembered and restored widths", () => {
    rememberNavigationWidth(310.4);
    expect(window.localStorage.getItem(navigationWidthStorageKey)).toBe("310");
    expect(detectNavigationWidth()).toBe(310);

    window.localStorage.setItem(navigationWidthStorageKey, "999");
    expect(detectNavigationWidth()).toBe(maximumNavigationWidth);
  });
});

describe("unavailable storage", () => {
  it("falls back without breaking the shell", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });

    expect(detectNavigationWidth()).toBe(defaultNavigationWidth);
    expect(() => rememberNavigationWidth(300)).not.toThrow();
  });
});
