import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { Tag } from "../ui/Tag";
import { presetName } from "./PresetPicker";
import { standardPresetId, useSettings } from "./settingsStore";

// SettingScope is how far a setting reaches. Every place that shows a setting says it the same way, so a person
// knows what a change touches before making it.
// - allMachines: one value for every machine and preset (metadata.json settings).
// - preset: the machines that use that preset.
// - eachMachine: a value kept apart for each machine, shown for all of them at once (the list of machines).
// - thisMachine: this machine only, and not synchronized.
export type SettingScope = "allMachines" | "preset" | "eachMachine" | "thisMachine";

const scopeLabels: Record<Exclude<SettingScope, "preset">, MessageKey> = {
  allMachines: "settings.scopeAllMachines",
  eachMachine: "settings.scopeEachMachine",
  thisMachine: "settings.scopeThisMachine",
};

// useSettingScopeLabel names a scope. A preset is named, since which preset decides which machines change;
// without presetId it is the one this machine uses, which is the one a setting outside Settings writes to.
export function useSettingScopeLabel(): (scope: SettingScope, presetId?: string) => string {
  const t = useTranslate();
  const settingsDocument = useSettings().document;
  return (scope, presetId) => {
    if (scope !== "preset") return t(scopeLabels[scope]);
    const id = presetId ?? settingsDocument?.presetInUse ?? standardPresetId;
    const preset = settingsDocument?.presets.find((candidate) => candidate.id === id);
    const name = preset === undefined ? t("settings.presetStandard") : presetName(preset, t("settings.presetStandard"));
    return t("settings.scopePreset", { name });
  };
}

export function SettingScopeTag({ scope, presetId }: { scope: SettingScope; presetId?: string }) {
  const label = useSettingScopeLabel();
  return <Tag>{label(scope, presetId)}</Tag>;
}
