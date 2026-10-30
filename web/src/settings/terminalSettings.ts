import type { components } from "../api/schema";
import { presetInUse, useSettings, type GlobalSettings, type SettingsDocument, type SettingsPreset } from "./settingsStore";

// The embedded terminal's settings as a terminal applies them: the preset's (how it looks and behaves, the
// limits, where a local shell starts) with the global ones (reconnects, the connection log, OSC 52).

export type PresetTerminalSettings = components["schemas"]["EmbeddedTerminal"];
export type GlobalTerminalSettings = components["schemas"]["GlobalTerminalSettings"];
export type TerminalSettings = PresetTerminalSettings & GlobalTerminalSettings;

// terminalSettingsOf is the terminal settings of preset together with the global ones.
function terminalSettingsOf(preset: SettingsPreset, global: GlobalSettings): TerminalSettings {
  return { ...preset.terminal, ...global.terminal };
}

function terminalSettingsInUse(document: SettingsDocument | null): TerminalSettings {
  return document === null ? {} : terminalSettingsOf(presetInUse(document), document.global);
}

// useTerminalSettings is the terminal settings this machine's terminals use.
export function useTerminalSettings(): TerminalSettings {
  return terminalSettingsInUse(useSettings().document);
}
