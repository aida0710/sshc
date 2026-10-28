import { useCallback } from "react";
import { settingsApi, type TerminalSettings } from "../api/settings";

// useOSC52Setting turns OSC 52 (a program writing to the clipboard) on or off for
// every terminal. There is no per-connection choice: the one setting is the one in
// Settings > Terminal.
export function useOSC52Setting({ settings, setSettings }: {
  settings: TerminalSettings;
  setSettings: (settings: TerminalSettings) => void;
}) {
  return useCallback(async (enabled: boolean): Promise<void> => {
    const next = { ...settings };
    if (enabled) next.osc52 = true;
    else delete next.osc52;
    await settingsApi.setTerminalSettings(next);
    setSettings(next);
  }, [settings, setSettings]);
}
