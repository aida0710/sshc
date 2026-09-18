import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { clampColumnWidth, readStoredColumnWidth, useStoredColumnWidth, type StoredColumnWidth } from "./useStoredColumnWidth";

const column: StoredColumnWidth = { key: "sshc.test.column-width", fallback: 240, minimum: 192, maximum: 384 };

afterEach(() => {
  vi.restoreAllMocks();
  window.localStorage.clear();
});

describe("a stored column width", () => {
  it("uses the fallback for missing and invalid values", () => {
    expect(readStoredColumnWidth(column)).toBe(column.fallback);
    for (const invalid of ["", "wide", "NaN", "Infinity"]) {
      window.localStorage.setItem(column.key, invalid);
      expect(readStoredColumnWidth(column)).toBe(column.fallback);
    }
  });

  it("rounds and clamps widths to the supported range", () => {
    expect(clampColumnWidth(260.6, column)).toBe(261);
    expect(clampColumnWidth(column.minimum - 100, column)).toBe(column.minimum);
    expect(clampColumnWidth(column.maximum + 100, column)).toBe(column.maximum);
    expect(clampColumnWidth(Number.NaN, column)).toBe(column.fallback);
  });

  it("normalises the width it remembers and the one it restores", () => {
    const { result } = renderHook(() => useStoredColumnWidth(column));
    act(() => result.current[1](310.4));
    expect(result.current[0]).toBe(310);
    expect(window.localStorage.getItem(column.key)).toBe("310");

    window.localStorage.setItem(column.key, "999");
    expect(readStoredColumnWidth(column)).toBe(column.maximum);
  });

  it("falls back without breaking the page when storage is blocked", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });

    const { result } = renderHook(() => useStoredColumnWidth(column));
    expect(result.current[0]).toBe(column.fallback);
    expect(() => act(() => result.current[1](300))).not.toThrow();
    expect(result.current[0]).toBe(300);
  });
});
