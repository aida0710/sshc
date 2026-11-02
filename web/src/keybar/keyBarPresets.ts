import type { components } from "../api/schema";
import { createPresetLibrary } from "../settings/presetLibrary";
import { newIdentifier } from "../ui/randomIdentifier";
import { defaultKeyBarLayout, type KeyBarLayout } from "./keyBarLayout";

export type KeyBarPreset = components["schemas"]["KeyBarPreset"];

const library = createPresetLibrary<KeyBarPreset>({
  endpoint: "/api/v1/metadata/keybar",
  read: (metadata) => metadata.keyBarPresets,
  choice: "keyBarPreset",
});

// useKeyBarPresets is the key bar presets and the one a settings preset chose (the one this machine uses when
// settingsPresetId is left out).
export const useKeyBarPresets = library.usePresets;

// useKeyBarLayout is the key bar of a settings preset (the one in force when settingsPresetId is left out): its
// chosen preset, or the default layout.
export function useKeyBarLayout(settingsPresetId?: string): KeyBarLayout {
  const chosen = library.useChosen(settingsPresetId);
  return chosen === undefined ? defaultKeyBarLayout : { keys: chosen.keys, moreKeys: chosen.moreKeys };
}

export const selectKeyBarPreset = library.select;
export const updateKeyBarPresets = library.update;

// saveKeyBarLayout keeps an edited layout in the key bar preset a settings preset chose, or in a new preset
// named newPresetName when it chose the default layout.
export function saveKeyBarLayout(layout: KeyBarLayout, newPresetName: string, settingsPresetId?: string) {
  return library.saveChosen((chosen) => chosen === undefined
    ? { id: newIdentifier(), name: newPresetName, ...layout }
    : { ...chosen, ...layout }, settingsPresetId);
}

