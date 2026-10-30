import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { fakeSettingsServer } from "../testing/settingsServer";
import { clampColumnWidth, usePresetColumnWidth, type PresetColumnWidth } from "./usePresetColumnWidth";

const column: PresetColumnWidth = { field: "navigationWidth", fallback: 240, minimum: 192, maximum: 384 };

afterEach(() => vi.restoreAllMocks());

describe("a column width kept in the preset", () => {
  it("rounds and clamps widths to the supported range", () => {
    expect(clampColumnWidth(260.6, column)).toBe(261);
    expect(clampColumnWidth(column.minimum - 100, column)).toBe(column.minimum);
    expect(clampColumnWidth(column.maximum + 100, column)).toBe(column.maximum);
    expect(clampColumnWidth(Number.NaN, column)).toBe(column.fallback);
  });

  it("uses the fallback until the preset holds a width, and clamps a stored one", () => {
    fakeSettingsServer({ presets: [{ id: "default", interface: { navigationWidth: 999 } }] });
    const { result } = renderHook(() => usePresetColumnWidth(column));

    expect(result.current[0]).toBe(column.maximum);
  });

  it("follows the pointer at once and saves the width where it rests", async () => {
    const server = fakeSettingsServer();
    const { result } = renderHook(() => usePresetColumnWidth(column));

    act(() => result.current[1](300.4));
    act(() => result.current[1](310.4));

    expect(result.current[0]).toBe(310);
    expect(server.writesTo("/api/v1/settings/presets/default")).toEqual([]);
    await waitFor(() => expect(server.document().presets[0]).toEqual({ id: "default", interface: { navigationWidth: 310 } }));
    expect(server.writesTo("/api/v1/settings/presets/default")).toHaveLength(1);
  });

  it("does not write the default width", async () => {
    const server = fakeSettingsServer({ presets: [{ id: "default", interface: { navigationWidth: 300 } }] });
    const { result } = renderHook(() => usePresetColumnWidth(column));

    act(() => result.current[1](column.fallback));

    await waitFor(() => expect(server.document().presets[0]).toEqual({ id: "default" }));
  });
});
