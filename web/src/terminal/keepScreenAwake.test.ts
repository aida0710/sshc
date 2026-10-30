import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { fakeSettingsServer } from "../testing/settingsServer";
import { saveKeepScreenAwake, useKeepScreenAwake } from "./keepScreenAwake";

afterEach(() => vi.restoreAllMocks());

describe("keepScreenAwake", () => {
  it("is off until it is turned on, and is kept in the preset this machine uses", async () => {
    const server = fakeSettingsServer();
    const { result } = renderHook(() => useKeepScreenAwake());
    expect(result.current).toBe(false);

    await act(async () => saveKeepScreenAwake(true));

    expect(result.current).toBe(true);
    await waitFor(() => expect(server.document().presets[0]).toEqual({ id: "default", interface: { keepScreenAwake: true } }));
  });

  it("removes the choice from the preset when it is turned off again", async () => {
    const server = fakeSettingsServer({ presets: [{ id: "default", interface: { keepScreenAwake: true } }] });
    const { result } = renderHook(() => useKeepScreenAwake());
    expect(result.current).toBe(true);

    await act(async () => saveKeepScreenAwake(false));

    await waitFor(() => expect(server.document().presets[0]).toEqual({ id: "default" }));
    expect(result.current).toBe(false);
  });
});
