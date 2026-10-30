import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { fakeSettingsServer } from "../testing/settingsServer";
import { useOSC52Setting } from "./useOSC52Setting";

afterEach(() => vi.restoreAllMocks());

describe("useOSC52Setting", () => {
  it("turns OSC 52 on in the global settings, keeping the other terminal settings", async () => {
    const server = fakeSettingsServer({ global: { terminal: { verbosity: 1 } } });
    const { result } = renderHook(() => useOSC52Setting());

    await result.current(true);

    await waitFor(() => expect(server.document().global).toEqual({ terminal: { verbosity: 1, osc52: true } }));
  });

  it("drops the setting when OSC 52 is turned off, as the default is off", async () => {
    const server = fakeSettingsServer({ global: { terminal: { osc52: true } } });
    const { result } = renderHook(() => useOSC52Setting());

    await result.current(false);

    await waitFor(() => expect(server.document().global).toEqual({}));
  });
});
