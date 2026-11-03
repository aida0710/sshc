import type { components } from "../api/schema";
import { createPresetLibrary, refreshPresetLibraries } from "../settings/presetLibrary";
import { newIdentifier } from "../ui/randomIdentifier";
import { defaultBindings, type Bindings } from "./bindings";

export type Preset = components["schemas"]["ShortcutPreset"];

export const shortcutPresetLibrary = createPresetLibrary<Preset>({
  endpoint: "/api/v1/metadata/shortcuts",
  read: (metadata) => metadata.shortcutPresets,
  choice: "shortcutPreset",
});

// bindingsOf is the shortcuts of a preset, or the defaults when a settings preset chose none or the one it
// chose is gone.
function bindingsOf(preset: Preset | undefined): Bindings {
  return preset === undefined ? defaultBindings : { ...defaultBindings, ...preset.bindings };
}

// usePresets is the shortcut presets and the one a settings preset chose (the one this machine uses when
// settingsPresetId is left out).
export const usePresets = shortcutPresetLibrary.usePresets;

// useBindings is the shortcuts of a settings preset (the ones in force when settingsPresetId is left out), for
// the screens that show or match them.
export function useBindings(settingsPresetId?: string): Bindings {
  return bindingsOf(shortcutPresetLibrary.useChosen(settingsPresetId));
}

// currentBindings is the shortcuts in force, for key handlers outside React.
export function currentBindings(): Bindings {
  return bindingsOf(shortcutPresetLibrary.currentChosen());
}

// refreshPresets reads the presets again, with every other preset library.
export const refreshPresets = refreshPresetLibraries;

// savePresetBindings keeps edited bindings in the shortcut preset a settings preset chose, or in a new preset
// named newPresetName when it chose the default bindings.
export function savePresetBindings(bindings: Bindings, newPresetName: string, settingsPresetId?: string) {
  return shortcutPresetLibrary.saveChosen((chosen) => chosen === undefined ? { id: newIdentifier(), name: newPresetName, bindings } : { ...chosen, bindings }, settingsPresetId);
}
