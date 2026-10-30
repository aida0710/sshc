import { currentSettings, presetInUse, updatePreset, useSettings, type InterfaceSettings, type SettingsDocument, type SettingsPreset } from "./settingsStore";

// The screen's own settings in a preset (InterfaceSettings): how terminals take touch, the notification sound,
// keeping the screen on, the shortcut preset, the column widths and the Home view. The engine never reads
// them. Where no preset id is given, they are those of the preset this machine uses.

// presetOf is the preset presetId names, or the one this machine uses when presetId is left out.
function presetOf(document: SettingsDocument | null, presetId?: string): SettingsPreset | undefined {
  return presetId === undefined ? presetInUse(document) : document?.presets.find((preset) => preset.id === presetId);
}

export function presetInterfaceOf(document: SettingsDocument | null, presetId?: string): InterfaceSettings {
  return presetOf(document, presetId)?.interface ?? {};
}

export function usePresetInterface(presetId?: string): InterfaceSettings {
  return presetInterfaceOf(useSettings().document, presetId);
}

// InterfaceChange is the screen's settings after a change; undefined removes a setting.
export type InterfaceChange = { [Key in keyof InterfaceSettings]?: InterfaceSettings[Key] | undefined };

// updatePresetInterface changes the screen's settings of a preset. A setting left at its default is removed
// rather than written, so a preset holds only what was chosen.
export function updatePresetInterface(change: (settings: InterfaceSettings) => InterfaceChange, presetId?: string): Promise<void> {
  return updatePreset(presetId ?? presetInUse(currentSettings()).id, (preset) => {
    const next = Object.fromEntries(
      Object.entries(change({ ...preset.interface })).filter(([, value]) => value !== undefined && value !== false && value !== 0 && value !== ""),
    ) as InterfaceSettings;
    const { interface: _previous, ...rest } = preset;
    return Object.keys(next).length === 0 ? rest : { ...rest, interface: next };
  });
}
