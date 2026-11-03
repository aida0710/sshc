import { useEffect, useSyncExternalStore } from "react";
import { apiClient } from "../api/client";
import { putJSON } from "../api/guards";
import type { Metadata } from "../api/config";
import { validateOpenAPISchema } from "../api/validators.generated";
import { usePolling } from "../ui/usePolling";
import { presetInterfaceOf, updatePresetInterface } from "./presetInterface";
import { currentSettings, useSettings, type InterfaceSettings, type SettingsDocument } from "./settingsStore";

// A preset library is a list of named presets synced in the metadata (the shortcut presets, the key bar
// presets), of which each settings preset chooses one by id. Every library is read from one request for the
// metadata and saved with the list it was read as, so an edit made meanwhile on another device is refused
// rather than overwritten.

// defaultPresetId stands for a library's defaults, which are not a saved preset.
export const defaultPresetId = "default";

// Presets edited on another synced device arrive on the next push; a few seconds keeps the libraries in step
// without a request per keystroke.
const presetPollIntervalMs = 5_000;

export type LibraryPreset = { id: string; name: string };

export type PresetLibraryState<Preset> = { presets: Preset[]; loading: boolean; busy: boolean; error: boolean };

export type PresetLibrary<Preset extends LibraryPreset> = {
  // usePresets is the library and the preset a settings preset chose (the one this machine uses when
  // settingsPresetId is left out). presets holds the saved ones only. A choice of a deleted preset is shown as the
  // defaults, which are in force.
  usePresets(settingsPresetId?: string): PresetLibraryState<Preset> & { selected: string };
  // useChosen is the preset in force for a settings preset, or undefined for the defaults.
  useChosen(settingsPresetId?: string): Preset | undefined;
  // currentChosen is the preset in force on this machine, for handlers outside React.
  currentChosen(): Preset | undefined;
  // select makes a settings preset take the preset id.
  select(id: string, settingsPresetId?: string): Promise<void>;
  // update replaces the list and makes a settings preset take selected.
  update(next: Preset[], selected: string, settingsPresetId?: string): Promise<void>;
  // saveChosen keeps an edit in the preset a settings preset chose; build is given undefined when it chose
  // the defaults or a built-in preset, neither of which can change, and returns a new preset to keep the edit in.
  saveChosen(build: (chosen: Preset | undefined) => Preset, settingsPresetId?: string): Promise<void>;
};

type ChoiceKey = "shortcutPreset" | "keyBarPreset";

type Registration = {
  // begin notes the library's state before the metadata is read; the returned function takes the metadata,
  // or null when the read failed, and keeps it only if nothing was saved meanwhile.
  begin(generation: number): ((metadata: Metadata | null) => void) | null;
  setLoading(): void;
};

const registrations = new Set<Registration>();
let generation = 0;
let refreshing = false;

export function createPresetLibrary<Preset extends LibraryPreset>({ endpoint, read, choice, builtIn = [] }: {
  endpoint: string;
  read: (metadata: Metadata) => Preset[] | undefined;
  choice: ChoiceKey;
  // builtIn are presets that ship with sshc: a settings preset chooses one by id like a saved one, but they are not
  // in the list that is saved.
  builtIn?: readonly Preset[];
}): PresetLibrary<Preset> {
  let state: PresetLibraryState<Preset> = { presets: [], loading: true, busy: false, error: false };
  let revision = 0;
  const listeners = new Set<() => void>();
  const publish = (patch: Partial<PresetLibraryState<Preset>>) => { state = { ...state, ...patch }; listeners.forEach((listener) => listener()); };
  const subscribe = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; };
  const snapshot = () => state;

  // chosenId is the choice as stored, which may name a preset deleted since.
  const chosenId = (document: SettingsDocument | null, settingsPresetId?: string) =>
    presetInterfaceOf(document, settingsPresetId)[choice] ?? defaultPresetId;
  const savedIn = (presets: Preset[], id: string) => presets.find((preset) => preset.id === id);
  const chosenIn = (presets: Preset[], id: string) => savedIn(presets, id) ?? builtIn.find((preset) => preset.id === id);

  registrations.add({
    begin(atGeneration) {
      if (state.busy) return null;
      const atRevision = revision;
      return (metadata) => {
        if (atGeneration !== generation) return;
        if (metadata === null) { publish({ loading: false, error: true }); return; }
        if (atRevision !== revision) return;
        publish({ presets: read(metadata) ?? [], loading: false, error: false });
      };
    },
    setLoading() { publish({ loading: true }); },
  });

  const select = (id: string, settingsPresetId?: string) =>
    updatePresetInterface((settings): InterfaceSettings => ({ ...settings, [choice]: id === defaultPresetId ? undefined : id }), settingsPresetId);

  async function update(next: Preset[], selected: string, settingsPresetId?: string) {
    if (state.busy || state.loading || state.error) throw new Error("Presets are not ready; reload before saving");
    ++revision;
    publish({ busy: true });
    try {
      await putJSON(endpoint, { base: state.presets, presets: next });
      publish({ presets: next });
    } catch (error) {
      publish({ error: true });
      throw error;
    } finally { publish({ busy: false }); }
    if (selected !== chosenId(currentSettings(), settingsPresetId)) await select(selected, settingsPresetId);
  }

  return {
    usePresets(settingsPresetId) {
      const library = useSyncExternalStore(subscribe, snapshot);
      const chosen = chosenId(useSettings().document, settingsPresetId);
      return { ...library, selected: chosenIn(library.presets, chosen) === undefined ? defaultPresetId : chosen };
    },
    useChosen(settingsPresetId) {
      const library = useSyncExternalStore(subscribe, snapshot);
      return chosenIn(library.presets, chosenId(useSettings().document, settingsPresetId));
    },
    currentChosen: () => chosenIn(state.presets, chosenId(currentSettings())),
    select,
    update,
    async saveChosen(build, settingsPresetId) {
      const saved = savedIn(state.presets, chosenId(currentSettings(), settingsPresetId));
      const preset = build(saved);
      const next = saved === undefined ? [...state.presets, preset] : state.presets.map((p) => p.id === preset.id ? preset : p);
      await update(next, preset.id, settingsPresetId);
    },
  };
}

// refreshPresetLibraries reads the metadata once and gives every library its list.
export async function refreshPresetLibraries() {
  if (refreshing) return;
  refreshing = true;
  const finishers = [...registrations].map((registration) => registration.begin(generation)).filter((finish) => finish !== null);
  try {
    const metadata = validateOpenAPISchema<Metadata>("Metadata", await apiClient.read("/api/v1/metadata"));
    finishers.forEach((finish) => finish(metadata));
  } catch { finishers.forEach((finish) => finish(null)); }
  finally { refreshing = false; }
}

// usePresetLibrarySync keeps every library in step with the metadata while the vault is open (ready).
export function usePresetLibrarySync(ready: boolean) {
  useEffect(() => {
    if (!ready) return;
    ++generation;
    registrations.forEach((registration) => registration.setLoading());
    void refreshPresetLibraries();
    const refresh = () => { void refreshPresetLibraries(); };
    window.addEventListener("focus", refresh);
    return () => { ++generation; window.removeEventListener("focus", refresh); };
  }, [ready]);
  usePolling(refreshPresetLibraries, { intervalMs: presetPollIntervalMs, enabled: ready, whileHidden: true });
}
