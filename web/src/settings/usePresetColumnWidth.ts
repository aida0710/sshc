import { useState } from "react";
import { presetInterfaceOf, updatePresetInterface } from "./presetInterface";
import { useSettings } from "./settingsStore";
import { useRestingSave } from "./useRestingSave";

export type PresetColumnWidth = {
  // field is where the preset keeps the width.
  field: "navigationWidth" | "connectionListWidth" | "connectionGroupsWidth";
  fallback: number;
  minimum: number;
  maximum: number;
};

// Width is a whole number of pixels inside [minimum, maximum]; anything
// unreadable becomes the fallback rather than a NaN-sized column.
export function clampColumnWidth(width: number, bounds: PresetColumnWidth): number {
  if (!Number.isFinite(width)) return bounds.fallback;
  return Math.min(bounds.maximum, Math.max(bounds.minimum, Math.round(width)));
}

// usePresetColumnWidth keeps one column width in the preset this machine uses. The width follows the pointer
// at once and is saved once it rests.
export function usePresetColumnWidth(bounds: PresetColumnWidth): [number, (width: number) => void] {
  const stored = presetInterfaceOf(useSettings().document)[bounds.field];
  const saved = stored === undefined ? bounds.fallback : clampColumnWidth(stored, bounds);
  // dragged is the width on screen while it has not been saved yet.
  const [dragged, setDragged] = useState<number | null>(null);
  const save = useRestingSave((width: number) => {
    // The default width is not written, so a preset keeps only a width someone chose.
    void updatePresetInterface((settings) => ({ ...settings, [bounds.field]: width === bounds.fallback ? undefined : width }))
      .catch(() => undefined)
      .finally(() => setDragged(null));
  });

  const change = (next: number) => {
    const clamped = clampColumnWidth(next, bounds);
    setDragged(clamped);
    save(clamped);
  };
  return [dragged ?? saved, change];
}
