import { useCallback } from "react";
import { updateGlobalSettings } from "../settings/settingsStore";

// useOSC52Setting turns OSC 52 (a program writing to the clipboard) on or off for
// every terminal. There is no per-connection choice: it is one of the global
// settings, the one in Settings > Terminal.
export function useOSC52Setting() {
  return useCallback((enabled: boolean): Promise<void> => updateGlobalSettings((settings) => {
    const { osc52: _previous, ...terminal } = settings.terminal ?? {};
    const next = enabled ? { ...terminal, osc52: true } : terminal;
    const { terminal: _terminal, ...rest } = settings;
    return Object.keys(next).length === 0 ? rest : { ...rest, terminal: next };
  }), []);
}
