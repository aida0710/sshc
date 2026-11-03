import type { components } from "../api/schema";
import { createPresetLibrary } from "../settings/presetLibrary";
import { newIdentifier } from "../ui/randomIdentifier";
import { claudeCodeKeyBarLayout, defaultKeyBarLayout, type KeyBarLayout } from "./keyBarLayout";

export type KeyBarPreset = components["schemas"]["KeyBarPreset"];

// builtInKeyBarPresets are the key bars sshc ships besides the default one. Only the choice of one is synced, in
// the settings preset: the list a person edits on every device stays their own.
export const builtInKeyBarPresets: readonly KeyBarPreset[] = [
  { id: "claude-code", name: "Claude Code", ...claudeCodeKeyBarLayout },
];

export const keyBarPresetLibrary = createPresetLibrary<KeyBarPreset>({
  endpoint: "/api/v1/metadata/keybar",
  read: (metadata) => metadata.keyBarPresets,
  choice: "keyBarPreset",
  builtIn: builtInKeyBarPresets,
});

// useKeyBarPresets is the key bar presets and the one a settings preset chose (the one this machine uses when
// settingsPresetId is left out).
export const useKeyBarPresets = keyBarPresetLibrary.usePresets;

// useKeyBarLayout is the key bar of a settings preset (the one in force when settingsPresetId is left out): its
// chosen preset, or the default layout.
export function useKeyBarLayout(settingsPresetId?: string): KeyBarLayout {
  const chosen = keyBarPresetLibrary.useChosen(settingsPresetId);
  return chosen === undefined ? defaultKeyBarLayout : { keys: chosen.keys, moreKeys: chosen.moreKeys };
}

// saveKeyBarLayout keeps an edited layout in the key bar preset a settings preset chose, or in a new preset
// named newPresetName when it chose the default layout or a built-in one.
export function saveKeyBarLayout(layout: KeyBarLayout, newPresetName: string, settingsPresetId?: string) {
  return keyBarPresetLibrary.saveChosen((chosen) => chosen === undefined
    ? { id: newIdentifier(), name: newPresetName, ...layout }
    : { ...chosen, ...layout }, settingsPresetId);
}

