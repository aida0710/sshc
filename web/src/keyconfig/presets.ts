import type { components } from "../api/schema";
import { createPresetLibrary, defaultPresetId, refreshPresetLibraries } from "../settings/presetLibrary";
import { newIdentifier } from "../ui/randomIdentifier";
import { defaultBindings, type Bindings } from "./bindings";

// defaultShortcutPresetId stands for the default bindings, which are not a saved preset.
export const defaultShortcutPresetId = defaultPresetId;

export type Preset = components["schemas"]["ShortcutPreset"];

const library = createPresetLibrary<Preset>({
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
export const usePresets = library.usePresets;

// useBindings is the shortcuts of a settings preset (the ones in force when settingsPresetId is left out), for
// the screens that show or match them.
export function useBindings(settingsPresetId?: string): Bindings {
  return bindingsOf(library.useChosen(settingsPresetId));
}

// currentBindings is the shortcuts in force, for key handlers outside React.
export function currentBindings(): Bindings {
  return bindingsOf(library.currentChosen());
}

// selectPreset makes a settings preset (the one this machine uses when settingsPresetId is left out) take the
// shortcut preset id.
export const selectPreset = library.select;

// refreshPresets reads the presets again, with every other preset library.
export const refreshPresets = refreshPresetLibraries;

// updatePresets replaces the list of shortcut presets and makes a settings preset take selected.
export const updatePresets = library.update;

// savePresetBindings keeps edited bindings in the shortcut preset a settings preset chose, or in a new one when
// it chose the default bindings.
export function savePresetBindings(bindings: Bindings, settingsPresetId?: string) {
  return library.saveChosen((chosen) => chosen === undefined ? { id: newIdentifier(), name: "My shortcuts", bindings } : { ...chosen, bindings }, settingsPresetId);
}
