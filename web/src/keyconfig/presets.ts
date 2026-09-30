import { useEffect, useSyncExternalStore } from "react";
import { usePolling } from "../ui/usePolling";
import { apiClient } from "../api/client";
import { putJSON } from "../api/guards";
import type { Metadata } from "../api/config";
import type { components } from "../api/schema";
import { validateOpenAPISchema } from "../api/validators.generated";
import { defaultBindings, loadBindings, saveBindings, type Bindings } from "./bindings";
import { readStoredValue, writeStoredValue } from "../ui/browserStorage";
import { localStorageKeys } from "../ui/browserStorageKeys";
import { newIdentifier } from "../ui/randomIdentifier";

// Presets edited on another synced device arrive on the next push; a few
// seconds keeps the two in step without a request per keystroke.
const presetPollIntervalMs = 5_000;

export type Preset = components["schemas"]["ShortcutPreset"];
type State = { presets: Preset[]; selected: string; loading: boolean; busy: boolean; error: boolean };
let state: State = { presets: [], selected: "default", loading: true, busy: false, error: false };
const listeners = new Set<() => void>();
let generation = 0;
let refreshing = false;
let revision = 0;
const publish = (patch: Partial<State>) => { state = { ...state, ...patch }; listeners.forEach((fn) => fn()); };
const subscribe = (fn: () => void) => { listeners.add(fn); return () => { listeners.delete(fn); }; };
export const usePresets = () => useSyncExternalStore(subscribe, () => state);

async function put(base: Preset[], presets: Preset[]) {
  await putJSON<unknown>("/api/v1/metadata/shortcuts", { base, presets });
}
// `remember` is false only while nothing has ever been chosen: a browser that
// is merely showing the defaults must not gain a stored preference for it.
// activate throws when the browser refuses to store the bindings, and then
// leaves the selection as it was, so the chosen preset never disagrees with
// the shortcuts in force.
function activate(id: string, presets: Preset[], remember = true) {
  const preset = presets.find((item) => item.id === id);
  const selected = preset ? id : "default";
  const bindings = preset?.bindings ?? defaultBindings;
  if (JSON.stringify(loadBindings()) !== JSON.stringify(bindings)) saveBindings(bindings);
  if (remember) writeStoredValue(localStorageKeys.shortcutPresetSelection, selected);
  publish({ selected });
}
export function selectPreset(id: string) {
  ++revision;
  activate(id, state.presets);
}
// A refresh runs from polling, focus and other tabs rather than from anything
// the user just did, so it does not report a refused storage.
function activateInBackground(id: string, presets: Preset[], remember: boolean) {
  try {
    activate(id, presets, remember);
  } catch {
    // The presets stay editable and this tab keeps its previous shortcuts.
    // The next refresh tries again, since the stored bindings still differ.
  }
}
export async function refreshPresets() {
  if (refreshing || state.busy) return;
  refreshing = true;
  const currentGeneration = generation;
  const currentRevision = revision;
  try {
    const metadata = validateOpenAPISchema<Metadata>("Metadata", await apiClient.read("/api/v1/metadata"));
    if (currentGeneration !== generation || currentRevision !== revision) return;
    const presets: Preset[] = metadata.shortcutPresets ?? [];
    const selected = readStoredValue(localStorageKeys.shortcutPresetSelection);
    activateInBackground(selected ?? "default", presets, selected !== null);
    publish({ presets, loading: false, error: false });
  } catch { if (currentGeneration === generation) publish({ loading: false, error: true }); }
  finally { refreshing = false; }
}
export async function updatePresets(next: Preset[], selected = state.selected) {
  if (state.busy || state.loading || state.error) throw new Error("Presets are not ready; reload before saving");
  ++revision;
  publish({ busy: true });
  try {
    await put(state.presets, next);
    publish({ presets: next });
    activate(selected, next);
  } catch (error) {
    publish({ error: true });
    throw error;
  } finally { publish({ busy: false }); }
}
export async function savePresetBindings(bindings: Bindings) {
  const selected = state.presets.find((p) => p.id === state.selected);
  const preset = selected ? { ...selected, bindings } : { id: newIdentifier(), name: "My shortcuts", bindings };
  await updatePresets(selected ? state.presets.map((p) => p.id === preset.id ? preset : p) : [...state.presets, preset], preset.id);
}
export function usePresetSync(ready: boolean) {
  useEffect(() => {
    if (!ready) return;
    ++generation;
    publish({ loading: true });
    void refreshPresets();
    const refresh = () => { void refreshPresets(); };
    window.addEventListener("focus", refresh);
    window.addEventListener("storage", refresh);
    return () => { ++generation; window.removeEventListener("focus", refresh); window.removeEventListener("storage", refresh); };
  }, [ready]);
  usePolling(refreshPresets, { intervalMs: presetPollIntervalMs, enabled: ready, whileHidden: true });
}
