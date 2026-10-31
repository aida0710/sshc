import { useEffect, useSyncExternalStore } from "react";
import { apiClient, failureCode } from "../api/client";
import { putJSON, sendJSON } from "../api/guards";
import type { components } from "../api/schema";
import { validateOpenAPISchema } from "../api/validators.generated";
import { usePolling } from "../ui/usePolling";

// The settings the engine keeps for every machine (GET /api/v1/settings): the global settings, the presets,
// the machines that sync and this machine's state. Every screen reads them from here, and every change is
// written through here, one at a time, so a change is never sent on top of a document the engine has not
// seen yet.

export type SettingsDocument = components["schemas"]["SettingsDocument"];
export type GlobalSettings = components["schemas"]["GlobalSettings"];
export type SettingsPreset = components["schemas"]["SettingsPreset"];
export type MachineSettings = components["schemas"]["MachineSettings"];
export type InterfaceSettings = components["schemas"]["InterfaceSettings"];
export type InterfaceState = components["schemas"]["InterfaceState"];
export type InterfaceStateKey = keyof InterfaceState;

// standardPresetId is the preset every machine uses until it chooses another (application.DefaultPresetID).
export const standardPresetId = "default";

// A setting changed on another synced machine arrives with the next sync; a few seconds keeps this browser in
// step without a request per keystroke.
const settingsPollIntervalMs = 5_000;

// settingsChangedCode is the engine's answer when what a change was based on moved underneath it.
const settingsChangedCode = "settings_changed";

type State = {
  document: SettingsDocument | null;
  // loadFailed is set when the first read failed; the screens then run on the defaults.
  loadFailed: boolean;
};

let state: State = { document: null, loadFailed: false };
const listeners = new Set<() => void>();
// writes chains every change, so each one reads its base after the one before it has landed.
let writes: Promise<unknown> = Promise.resolve();
// revision counts the changes made here; a read that started before one of them is dropped, so it never
// takes back what this browser just chose.
let revision = 0;
let refreshing = false;

function publish(next: State) {
  state = next;
  listeners.forEach((listener) => listener());
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
}

export function useSettings(): State {
  return useSyncExternalStore(subscribe, () => state);
}

// currentSettings is the document as this browser knows it, for code outside React.
export function currentSettings(): SettingsDocument | null {
  return state.document;
}

// presetInUse is the preset this machine uses: the one the engine resolved, or the standard preset.
export function presetInUse(document: SettingsDocument | null): SettingsPreset {
  if (document === null) return { id: standardPresetId };
  return document.presets.find((preset) => preset.id === document.presetInUse)
    ?? document.presets.find((preset) => preset.id === standardPresetId)
    ?? { id: standardPresetId };
}

async function readDocument(): Promise<SettingsDocument> {
  return validateOpenAPISchema<SettingsDocument>("SettingsDocument", await apiClient.read("/api/v1/settings"));
}

export async function refreshSettings(): Promise<void> {
  if (refreshing) return;
  refreshing = true;
  const startedRevision = revision;
  try {
    const document = await readDocument();
    // An unchanged document is not published again, so the screens keep the objects they hold and draw nothing
    // anew every few seconds.
    if (startedRevision === revision && !sameJSON(document, state.document)) publish({ document, loadFailed: false });
  } catch {
    if (state.document === null) publish({ document: null, loadFailed: true });
  } finally {
    refreshing = false;
  }
}

// useSettingsSync reads the settings once the engine is ready and keeps them current. It returns whether the
// first read has finished (or failed), so the app shell can wait for it instead of drawing the defaults first.
export function useSettingsSync(enabled: boolean): boolean {
  useEffect(() => {
    if (!enabled) return;
    void refreshSettings();
    const refresh = () => { void refreshSettings(); };
    window.addEventListener("focus", refresh);
    return () => window.removeEventListener("focus", refresh);
  }, [enabled]);
  usePolling(refreshSettings, { intervalMs: settingsPollIntervalMs, enabled, whileHidden: true });
  const current = useSettings();
  return current.document !== null || current.loadFailed;
}

function enqueue<T>(write: () => Promise<T>): Promise<T> {
  const next = writes.then(write, write);
  writes = next.catch(() => undefined);
  return next;
}

// writeWithRetry shows the change at once, sends it, and when the engine says the base moved (another machine
// or tab changed the same settings), reads the settings again and redoes the change once on top of them.
async function writeWithRetry(change: (document: SettingsDocument) => { next: SettingsDocument; send: () => Promise<unknown> } | null) {
  for (let attempt = 0; attempt < 2; attempt += 1) {
    if (state.document === null) await refreshSettings();
    const document = state.document;
    if (document === null) throw new Error("settings_unavailable");
    const planned = change(document);
    if (planned === null) return;
    revision += 1;
    publish({ ...state, document: planned.next });
    try {
      await planned.send();
      return;
    } catch (error) {
      revision += 1;
      const document = await readDocument().catch(() => null);
      publish({ ...state, document: document ?? state.document });
      if (failureCode(error) !== settingsChangedCode || attempt > 0) throw error;
    }
  }
}

function sameJSON(first: unknown, second: unknown): boolean {
  return JSON.stringify(first) === JSON.stringify(second);
}

// SettingsPart is one part of the document a change is written to: the global settings, the presets, one preset
// or one machine's settings.
type SettingsPart<T> = {
  // read is the part as this browser knows it, or undefined when it is gone and nothing is to be sent.
  read: (document: SettingsDocument) => T | undefined;
  // place is the document with next in place of the part.
  place: (document: SettingsDocument, next: T) => SettingsDocument;
  // send writes next with the base it was made from; the engine refuses with settingsChangedCode when the base
  // moved underneath it.
  send: (base: T, next: T) => Promise<unknown>;
};

// changePart makes the next value of a part from a copy of it and writes it after the changes before it. A
// change that leaves the part as it was sends nothing.
function changePart<T>(part: SettingsPart<T>, change: (current: T) => T): Promise<void> {
  return enqueue(() => writeWithRetry((document) => {
    const base = part.read(document);
    if (base === undefined) return null;
    const next = change(structuredClone(base));
    if (sameJSON(base, next)) return null;
    return { next: part.place(document, next), send: () => part.send(base, next) };
  }));
}

// updatePreset changes one preset's settings. The preset's id and name are changed with updatePresets.
export function updatePreset(id: string, change: (preset: SettingsPreset) => SettingsPreset): Promise<void> {
  return changePart({
    read: (document) => document.presets.find((preset) => preset.id === id),
    place: (document, preset) => ({ ...document, presets: document.presets.map((candidate) => candidate.id === id ? preset : candidate) }),
    send: (base, preset) => putJSON(`/api/v1/settings/presets/${encodeURIComponent(id)}`, { base, preset }, [settingsChangedCode]),
  }, change);
}

// updatePresetInUse changes the settings of the preset this machine uses.
export function updatePresetInUse(change: (preset: SettingsPreset) => SettingsPreset): Promise<void> {
  return updatePreset(presetInUse(state.document).id, change);
}

// updatePresets replaces the list of presets: adding, renaming, removing or reordering them.
export function updatePresets(change: (presets: SettingsPreset[]) => SettingsPreset[]): Promise<void> {
  return changePart({
    read: (document) => document.presets,
    place: (document, presets) => ({
      ...document,
      presets,
      presetInUse: presets.some((preset) => preset.id === document.presetInUse) ? document.presetInUse : standardPresetId,
    }),
    send: (base, presets) => putJSON("/api/v1/settings/presets", { base, presets }, [settingsChangedCode]),
  }, change);
}

export function updateGlobalSettings(change: (settings: GlobalSettings) => GlobalSettings): Promise<void> {
  return changePart({
    read: (document) => document.global,
    place: (document, settings) => ({ ...document, global: settings }),
    send: (base, settings) => putJSON("/api/v1/settings/global", { base, settings }, [settingsChangedCode]),
  }, change);
}

// updateMachine changes a machine's settings: its name, the preset it uses and its port.
export function updateMachine(id: string, change: (settings: MachineSettings) => MachineSettings): Promise<void> {
  return changePart({
    read: (document) => document.machines.find((candidate) => candidate.id === id)?.settings,
    place: (document, settings) => ({
      ...document,
      presetInUse: id === document.machineId
        ? (document.presets.some((preset) => preset.id === settings.presetId) ? settings.presetId ?? standardPresetId : standardPresetId)
        : document.presetInUse,
      machines: document.machines.map((candidate) => candidate.id === id ? { ...candidate, settings } : candidate),
    }),
    send: (base, settings) => putJSON(`/api/v1/settings/machines/${encodeURIComponent(id)}`, { base, settings }, [settingsChangedCode]),
  }, change);
}

// removeMachine takes a machine that is no longer used off the list.
export function removeMachine(id: string): Promise<void> {
  return enqueue(async () => {
    await apiClient.mutate<unknown>(`/api/v1/settings/machines/${encodeURIComponent(id)}`, { method: "DELETE" });
    revision += 1;
    if (state.document !== null) {
      publish({ ...state, document: { ...state.document, machines: state.document.machines.filter((machine) => machine.id !== id) } });
    }
  });
}

// setInterfaceState keeps one piece of this machine's screen layout (an empty value removes it). It shows at
// once; the engine writes it a little later together with the others, so a click never waits for it.
export function setInterfaceState(key: InterfaceStateKey, value: string): Promise<void> {
  const document = state.document;
  if (document === null || (document.state.interface?.[key] ?? "") === value) return Promise.resolve();
  const next = { ...document.state.interface };
  if (value === "") delete next[key];
  else next[key] = value;
  revision += 1;
  publish({ ...state, document: { ...document, state: { ...document.state, interface: next } } });
  return enqueue(() => sendJSON("/api/v1/settings/state", { method: "PUT", body: { interface: { [key]: value } } }));
}

// setTransfersStopped says whether this machine's transfer queue starts the transfers waiting in it. The engine
// writes it at once and applies it to the queue, which reports it in its next listing.
export function setTransfersStopped(stopped: boolean): Promise<void> {
  return enqueue(async () => {
    await sendJSON("/api/v1/settings/state", { method: "PUT", body: { transfersStopped: stopped } });
    const document = state.document;
    if (document === null) return;
    revision += 1;
    publish({ ...state, document: { ...document, state: { ...document.state, transfersStopped: stopped } } });
  });
}

// interfaceState reads one piece of this machine's screen layout, or "" when nothing is kept.
export function interfaceState(document: SettingsDocument | null, key: InterfaceStateKey): string {
  return document?.state.interface?.[key] ?? "";
}

// resetSettingsForTests forgets the document, for tests that start each case from nothing.
export function resetSettingsForTests(document: SettingsDocument | null = null): void {
  writes = Promise.resolve();
  revision += 1;
  publish({ document, loadFailed: false });
}
