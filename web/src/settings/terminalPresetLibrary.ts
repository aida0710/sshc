import { useEffect, useSyncExternalStore } from "react";
import { apiClient } from "../api/client";
import type { Metadata } from "../api/config";
import { putJSON } from "../api/guards";
import { validateOpenAPISchema } from "../api/validators.generated";
import { readStoredValue, writeStoredValue } from "../ui/browserStorage";
import { localStorageKeys } from "../ui/browserStorageKeys";
import { usePolling } from "../ui/usePolling";
import { standardPresetId, type TerminalPreset } from "./terminalPresets";

// The terminal presets of this machine's engine and the one this browser uses.
// It follows keyconfig/presets.ts (the shortcut presets) in how it reads and
// saves the list, but chooses nothing on its own: the terminal reads the chosen
// preset when it draws, where the shortcuts must be copied into the keys in force.

// A preset changed on another synced machine arrives with the next sync; a few
// seconds keeps this browser in step without a request per keystroke.
const presetPollIntervalMs = 5_000;

type State = { presets: TerminalPreset[]; selected: string; loading: boolean; busy: boolean; error: boolean };
let state: State = { presets: [], selected: standardPresetId, loading: true, busy: false, error: false };
const listeners = new Set<() => void>();
// generation drops a read that a newer start of the sync replaced; revision drops one
// that a save or a choice made after it started would undo.
let generation = 0;
let revision = 0;
let refreshing = false;
const publish = (patch: Partial<State>) => { state = { ...state, ...patch }; listeners.forEach((listener) => listener()); };
const subscribe = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; };

export const useTerminalPresets = () => useSyncExternalStore(subscribe, () => state);

// presetInUse is the preset this browser uses, or undefined for the standard set,
// also when the chosen preset was deleted on another machine.
export function presetInUse(library: Pick<State, "presets" | "selected">): TerminalPreset | undefined {
  return library.presets.find((preset) => preset.id === library.selected);
}

function storedSelection(): string {
  return readStoredValue(localStorageKeys.terminalPresetSelection) ?? standardPresetId;
}

// selectTerminalPreset makes this browser use the preset id (standardPresetId for the
// standard set). It throws when the browser refuses to store the choice.
export function selectTerminalPreset(id: string) {
  ++revision;
  writeStoredValue(localStorageKeys.terminalPresetSelection, id);
  publish({ selected: id });
}

export async function refreshTerminalPresets() {
  if (refreshing || state.busy) return;
  refreshing = true;
  const startedGeneration = generation;
  const startedRevision = revision;
  try {
    const metadata = validateOpenAPISchema<Metadata>("Metadata", await apiClient.read("/api/v1/metadata"));
    if (startedGeneration !== generation || startedRevision !== revision) return;
    publish({ presets: metadata.terminalPresets ?? [], selected: storedSelection(), loading: false, error: false });
  } catch {
    if (startedGeneration === generation) publish({ loading: false, error: true });
  } finally {
    refreshing = false;
  }
}

// updateTerminalPresets replaces the list. The engine refuses it when the list
// changed since this browser read it; the list then needs a reload.
export async function updateTerminalPresets(next: TerminalPreset[]) {
  if (state.busy || state.loading || state.error) throw new Error("Terminal presets are not ready; reload before saving");
  ++revision;
  publish({ busy: true });
  try {
    await putJSON<unknown>("/api/v1/metadata/terminal-presets", { base: state.presets, presets: next });
    publish({ presets: next });
  } catch (error) {
    publish({ error: true });
    throw error;
  } finally {
    publish({ busy: false });
  }
}

export function useTerminalPresetSync(ready: boolean) {
  useEffect(() => {
    if (!ready) return;
    ++generation;
    publish({ loading: true });
    void refreshTerminalPresets();
    const refresh = () => { void refreshTerminalPresets(); };
    window.addEventListener("focus", refresh);
    // Another tab of this browser chose a preset or saved the list.
    window.addEventListener("storage", refresh);
    return () => {
      ++generation;
      window.removeEventListener("focus", refresh);
      window.removeEventListener("storage", refresh);
    };
  }, [ready]);
  usePolling(refreshTerminalPresets, { intervalMs: presetPollIntervalMs, enabled: ready, whileHidden: true });
}
