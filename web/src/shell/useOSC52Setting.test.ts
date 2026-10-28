import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { settingsApi } from "../api/settings";
import { useOSC52Setting } from "./useOSC52Setting";

vi.mock("../api/settings", async () => {
  const actual = await vi.importActual<typeof import("../api/settings")>("../api/settings");
  return { ...actual, settingsApi: { ...actual.settingsApi, setTerminalSettings: vi.fn() } };
});

describe("useOSC52Setting", () => {
  it("turns OSC 52 on for every terminal, keeping the other terminal settings", async () => {
    vi.mocked(settingsApi.setTerminalSettings).mockResolvedValue(undefined);
    const setSettings = vi.fn();
    const { result } = renderHook(() => useOSC52Setting({ settings: { fontSize: 15 }, setSettings }));

    await result.current(true);

    expect(settingsApi.setTerminalSettings).toHaveBeenCalledWith({ fontSize: 15, osc52: true });
    expect(setSettings).toHaveBeenCalledWith({ fontSize: 15, osc52: true });
  });

  it("drops the setting when OSC 52 is turned off, as the default is off", async () => {
    vi.mocked(settingsApi.setTerminalSettings).mockResolvedValue(undefined);
    const setSettings = vi.fn();
    const { result } = renderHook(() => useOSC52Setting({ settings: { osc52: true }, setSettings }));

    await result.current(false);

    expect(setSettings).toHaveBeenCalledWith({});
  });
});
