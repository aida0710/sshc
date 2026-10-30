import { useEffect, useSyncExternalStore } from "react";
import { usePolling } from "../ui/usePolling";
import { apiClient } from "../api/client";
import { putJSON } from "../api/guards";
import type { Metadata } from "../api/config";
import type { components } from "../api/schema";
import { validateOpenAPISchema } from "../api/validators.generated";
import { presetInterfaceOf, updatePresetInterface } from "../settings/presetInterface";
import { currentSettings, useSettings, type SettingsDocument } from "../settings/settingsStore";
import { defaultBindings, type Bindings } from "./bindings";
import { newIdentifier } from "../ui/randomIdentifier";

// Presets edited on another synced device arrive on the next push; a few
// seconds keeps the two in step without a request per keystroke.
const presetPollIntervalMs = 5_000;

// defaultShortcutPresetId stands for the default bindings, which are not a saved preset.
export const defaultShortcutPresetId = "default";

export type Preset = components["schemas"]["ShortcutPreset"];
type State = { presets: Preset[]; loading: boolean; busy: boolean; error: boolean };
let state: State = { presets: [], loading: true, busy: false, error: false };
const listeners = new Set<() => void>();
let generation = 0;
let refreshing = false;
let revision = 0;
const publish = (patch: Partial<State>) => { state = { ...state, ...patch }; listeners.forEach((fn) => fn()); };
const subscribe = (fn: () => void) => { listeners.add(fn); return () => { listeners.delete(fn); }; };

// chosenShortcutPreset is the shortcut preset a settings preset chose (the one this machine uses when
// settingsPresetId is left out), or the default bindings. It is the choice as stored, which may name a shortcut
// preset deleted since.
function chosenShortcutPreset(document: SettingsDocument | null, settingsPresetId?: string): string {
  return presetInterfaceOf(document, settingsPresetId).shortcutPreset ?? defaultShortcutPresetId;
}

// bindingsOf is the shortcuts in force: those of the chosen shortcut preset, or the defaults when it chose
// none or the one it chose is gone.
function bindingsOf(presets: Preset[], selected: string): Bindings {
  const bindings = presets.find((preset) => preset.id === selected)?.bindings;
  return bindings === undefined ? defaultBindings : { ...defaultBindings, ...bindings };
}

// usePresets is the shortcut presets and the one a settings preset chose (the one this machine uses when
// settingsPresetId is left out).
export function usePresets(settingsPresetId?: string): State & { selected: string } {
  const library = useSyncExternalStore(subscribe, () => state);
  const chosen = chosenShortcutPreset(useSettings().document, settingsPresetId);
  // A choice of a deleted shortcut preset is shown as the default bindings, which are the ones in force.
  const selected = library.presets.some((preset) => preset.id === chosen) ? chosen : defaultShortcutPresetId;
  return { ...library, selected };
}

// useBindings is the shortcuts of a settings preset (the ones in force when settingsPresetId is left out), for
// the screens that show or match them.
export function useBindings(settingsPresetId?: string): Bindings {
  const library = useSyncExternalStore(subscribe, () => state);
  return bindingsOf(library.presets, chosenShortcutPreset(useSettings().document, settingsPresetId));
}

// currentBindings is the shortcuts in force, for key handlers outside React.
export function currentBindings(): Bindings {
  return bindingsOf(state.presets, chosenShortcutPreset(currentSettings()));
}

async function put(base: Preset[], presets: Preset[]) {
  await putJSON("/api/v1/metadata/shortcuts", { base, presets });
}

// selectPreset makes a settings preset (the one this machine uses when settingsPresetId is left out) take the
// shortcut preset id.
export function selectPreset(id: string, settingsPresetId?: string): Promise<void> {
  return updatePresetInterface((settings) => ({ ...settings, shortcutPreset: id === defaultShortcutPresetId ? undefined : id }), settingsPresetId);
}

export async function refreshPresets() {
  if (refreshing || state.busy) return;
  refreshing = true;
  const currentGeneration = generation;
  const currentRevision = revision;
  try {
    const metadata = validateOpenAPISchema<Metadata>("Metadata", await apiClient.read("/api/v1/metadata"));
    if (currentGeneration !== generation || currentRevision !== revision) return;
    publish({ presets: metadata.shortcutPresets ?? [], loading: false, error: false });
  } catch { if (currentGeneration === generation) publish({ loading: false, error: true }); }
  finally { refreshing = false; }
}

// updatePresets replaces the list of shortcut presets and makes a settings preset (the one this machine uses
// when settingsPresetId is left out) take selected.
export async function updatePresets(next: Preset[], selected: string, settingsPresetId?: string) {
  if (state.busy || state.loading || state.error) throw new Error("Presets are not ready; reload before saving");
  ++revision;
  publish({ busy: true });
  try {
    await put(state.presets, next);
    publish({ presets: next });
  } catch (error) {
    publish({ error: true });
    throw error;
  } finally { publish({ busy: false }); }
  if (selected !== chosenShortcutPreset(currentSettings(), settingsPresetId)) await selectPreset(selected, settingsPresetId);
}

// savePresetBindings keeps edited bindings in the shortcut preset a settings preset chose, or in a new one when
// it chose the default bindings.
export async function savePresetBindings(bindings: Bindings, settingsPresetId?: string) {
  const selectedId = chosenShortcutPreset(currentSettings(), settingsPresetId);
  const selected = state.presets.find((p) => p.id === selectedId);
  const preset = selected ? { ...selected, bindings } : { id: newIdentifier(), name: "My shortcuts", bindings };
  await updatePresets(selected ? state.presets.map((p) => p.id === preset.id ? preset : p) : [...state.presets, preset], preset.id, settingsPresetId);
}

export function usePresetSync(ready: boolean) {
  useEffect(() => {
    if (!ready) return;
    ++generation;
    publish({ loading: true });
    void refreshPresets();
    const refresh = () => { void refreshPresets(); };
    window.addEventListener("focus", refresh);
    return () => { ++generation; window.removeEventListener("focus", refresh); };
  }, [ready]);
  usePolling(refreshPresets, { intervalMs: presetPollIntervalMs, enabled: ready, whileHidden: true });
}
