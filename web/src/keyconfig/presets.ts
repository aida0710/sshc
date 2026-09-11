import { useEffect, useSyncExternalStore } from "react";
import { apiClient } from "../api/client";
import type { Metadata } from "../api/config";
import { validateOpenAPISchema } from "../api/validators.generated";
import { defaultBindings, loadBindings, parseBindings, saveBindings, storageKey, type Bindings } from "./bindings";

export type Preset = { id: string; name: string; bindings: Bindings };
export const selectionKey = "sshc.shortcuts.selected.v1";
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
  await apiClient.mutate("/api/v1/metadata/shortcuts", {
    method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ base, presets }),
  });
}
function activate(id: string, presets: Preset[]) {
  const preset = presets.find((item) => item.id === id);
  const selected = preset ? id : "default";
  const bindings = preset?.bindings ?? defaultBindings;
  window.localStorage.setItem(selectionKey, selected);
  if (JSON.stringify(loadBindings()) !== JSON.stringify(bindings)) saveBindings(bindings);
  publish({ selected });
}
export function selectPreset(id: string) {
  ++revision;
  activate(id, state.presets);
}
export async function refreshPresets() {
  if (refreshing || state.busy) return;
  refreshing = true;
  const currentGeneration = generation;
  const currentRevision = revision;
  try {
    const metadata = validateOpenAPISchema<Metadata>("Metadata", await apiClient.read("/api/v1/metadata"));
    if (currentGeneration !== generation || currentRevision !== revision) return;
    let presets: Preset[] = metadata.shortcutPresets ?? [];
    let selected = window.localStorage.getItem(selectionKey);
    // Persist the migration ID before writing so retries never create duplicates.
    if (selected === null) {
      selected = window.localStorage.getItem(storageKey) === null ? "default" : `pending:${crypto.randomUUID()}`;
      window.localStorage.setItem(selectionKey, selected);
    }
    if (selected.startsWith("pending:")) {
      const id = selected.slice(8);
      if (!presets.some((item) => item.id === id)) {
        const next = [...presets, { id, name: "Imported shortcuts", bindings: parseBindings(window.localStorage.getItem(storageKey) ?? "") }];
        await put(presets, next);
        if (currentGeneration !== generation || currentRevision !== revision) return;
        presets = next;
      }
      selected = id;
    }
    activate(selected, presets);
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
  const preset = selected ? { ...selected, bindings } : { id: crypto.randomUUID(), name: "My shortcuts", bindings };
  await updatePresets(selected ? state.presets.map((p) => p.id === preset.id ? preset : p) : [...state.presets, preset], preset.id);
}
export function usePresetSync(ready: boolean) {
  useEffect(() => {
    if (!ready) return;
    ++generation;
    publish({ loading: true });
    void refreshPresets();
    const timer = window.setInterval(() => { void refreshPresets(); }, 5000);
    const refresh = () => { void refreshPresets(); };
    window.addEventListener("focus", refresh);
    window.addEventListener("storage", refresh);
    return () => { ++generation; window.clearInterval(timer); window.removeEventListener("focus", refresh); window.removeEventListener("storage", refresh); };
  }, [ready]);
}
