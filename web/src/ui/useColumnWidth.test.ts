import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { clampColumnWidth, useColumnWidth, type ColumnWidth } from "./useColumnWidth";

const column: ColumnWidth = { key: "sshc.test.width", fallback: 240, minimum: 192, maximum: 384 };

afterEach(() => window.localStorage.clear());

describe("a column width kept in the browser", () => {
  it("rounds and clamps widths to the supported range", () => {
    expect(clampColumnWidth(260.6, column)).toBe(261);
    expect(clampColumnWidth(column.minimum - 100, column)).toBe(column.minimum);
    expect(clampColumnWidth(column.maximum + 100, column)).toBe(column.maximum);
    expect(clampColumnWidth(Number.NaN, column)).toBe(column.fallback);
  });

  it("uses the fallback until a width is stored, and clamps a stored one", () => {
    expect(renderHook(() => useColumnWidth(column)).result.current[0]).toBe(column.fallback);

    window.localStorage.setItem(column.key, "999");
    expect(renderHook(() => useColumnWidth(column)).result.current[0]).toBe(column.maximum);

    window.localStorage.setItem(column.key, "wide");
    expect(renderHook(() => useColumnWidth(column)).result.current[0]).toBe(column.fallback);
  });

  it("follows the pointer and keeps the width it ends at", () => {
    const { result } = renderHook(() => useColumnWidth(column));

    act(() => result.current[1](300.4));
    act(() => result.current[1](310.4));

    expect(result.current[0]).toBe(310);
    expect(window.localStorage.getItem(column.key)).toBe("310");
  });

  it("forgets the width when it goes back to the default", () => {
    window.localStorage.setItem(column.key, "300");
    const { result } = renderHook(() => useColumnWidth(column));

    act(() => result.current[1](column.fallback));

    expect(result.current[0]).toBe(column.fallback);
    expect(window.localStorage.getItem(column.key)).toBeNull();
  });
});
