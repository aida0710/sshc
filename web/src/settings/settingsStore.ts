import { useEffect, useLayoutEffect, useRef, useSyncExternalStore } from "react";
import { engineApiBase } from "../api/remoteEngines";
import { usePolling } from "../ui/usePolling";
import {
  createSettingsDocumentStore,
  standardPresetId,
  type GlobalSettings,
  type InterfaceStateKey,
  type MachineSettings,
  type SettingsDocument,
  type SettingsDocumentStore,
  type SettingsPreset,
  type SettingsState,
} from "./settingsDocumentStore";

export {
  standardPresetId,
  type GlobalSettings,
  type InterfaceSettings,
  type InterfaceState,
  type InterfaceStateKey,
  type MachineSettings,
  type SettingsDocument,
  type SettingsPreset,
} from "./settingsDocumentStore";

// 画面が描く設定と、このマシンの画面の状態。
//
// 画面は、選んでいるリモートセッションのエンジンの設定で描く（テーマ、言語、フォント、ターミナル、プリセット）。
// 設定の画面で変えたものも、そのエンジンへ書く。どれを選んでいるか（runTarget）と、このブラウザの画面の並び
// （interface: SFTP のタブ、たたんだワークスペースなど）は、どれを選んでいても、このマシンのエンジンに残す。

// A setting changed on another synced machine arrives with the next sync; a few seconds keeps this browser in
// step without a request per keystroke.
const settingsPollIntervalMs = 5_000;

// thisMachineSettings は、このマシンのエンジンの設定。リモートセッションを選んでいないときは、画面もこれで描く。
const thisMachineSettings = createSettingsDocumentStore(engineApiBase(null));
// remoteSettings は、リモートセッションのエンジンの設定。一度読んだものは残し、切り替えて戻ったときにすぐ描く。
const remoteSettings = new Map<string, SettingsDocumentStore>();
// shownRunTarget は、画面が描く設定のエンジン。null はこのマシン。
let shownRunTarget: string | null = null;
const shownListeners = new Set<() => void>();

function settingsOf(runTarget: string | null): SettingsDocumentStore {
  if (runTarget === null) return thisMachineSettings;
  let store = remoteSettings.get(runTarget);
  if (store === undefined) {
    store = createSettingsDocumentStore(engineApiBase(runTarget));
    remoteSettings.set(runTarget, store);
  }
  return store;
}

function shownSettings(): SettingsDocumentStore {
  return settingsOf(shownRunTarget);
}

// subscribeShown は、画面が描く設定の変わり目と、その設定の中身の変わり目の両方を知らせる。
function subscribeShown(listener: () => void) {
  let unsubscribe = shownSettings().subscribe(listener);
  const switched = () => {
    unsubscribe();
    unsubscribe = shownSettings().subscribe(listener);
    listener();
  };
  shownListeners.add(switched);
  return () => {
    shownListeners.delete(switched);
    unsubscribe();
  };
}

// showSettingsOf は、画面が描く設定を、runTarget のエンジンのもの（null はこのマシン）にする。読むのは
// useSettingsSync が行う。書きかけの変更は、始めたときのエンジンへ送る。
export function showSettingsOf(runTarget: string | null): void {
  if (runTarget === shownRunTarget) return;
  shownRunTarget = runTarget;
  shownListeners.forEach((listener) => listener());
}

// useSettings は、画面が描く設定（選んでいるリモートセッションのエンジンのもの）。
export function useSettings(): SettingsState {
  return useSyncExternalStore(subscribeShown, () => shownSettings().snapshot());
}

// useThisMachineSettings は、このマシンのエンジンの設定。このマシンの画面の状態（interfaceState）を読むのに使う。
export function useThisMachineSettings(): SettingsState {
  return useSyncExternalStore(thisMachineSettings.subscribe, thisMachineSettings.snapshot);
}

// currentSettings is the document the screens are drawn with, for code outside React.
export function currentSettings(): SettingsDocument | null {
  return shownSettings().snapshot().document;
}

// currentThisMachineSettings は、このマシンのエンジンの設定。React の外で、このマシンの画面の状態を読むのに使う。
export function currentThisMachineSettings(): SettingsDocument | null {
  return thisMachineSettings.snapshot().document;
}

// presetInUse is the preset this machine uses: the one the engine resolved, or the standard preset.
export function presetInUse(document: SettingsDocument | null): SettingsPreset {
  if (document === null) return { id: standardPresetId };
  return document.presets.find((preset) => preset.id === document.presetInUse)
    ?? document.presets.find((preset) => preset.id === standardPresetId)
    ?? { id: standardPresetId };
}

// refreshSettings は、このマシンの設定と、画面が描く設定（リモートセッションのもの）を読み直す。
export async function refreshSettings(): Promise<void> {
  await Promise.all([thisMachineSettings.refresh(), shownRunTarget === null ? undefined : shownSettings().refresh()]);
}

function isRead(state: SettingsState): boolean {
  return state.document !== null || state.loadFailed;
}

// useSettingsSync は、エンジンが使えるようになったら設定を読み、そのあとも読み直し続ける。runTarget は、画面が描く
// 設定のエンジン（選んでいるリモートセッション）で、描く前に切り替える。返すのは、起動してから、このマシンの設定と
// runTarget の設定を一度読み終えた（または読めなかった）か。アプリの枠は、最初の画面を既定の値で先に描かずにこれを
// 待つ。一度読み終えたら、リモートセッションを切り替えても待たない（画面を作り直さない）。
export function useSettingsSync(enabled: boolean, runTarget: string | null = null): boolean {
  useLayoutEffect(() => showSettingsOf(runTarget), [runTarget]);
  useEffect(() => {
    if (!enabled) return;
    void refreshSettings();
    const refresh = () => { void refreshSettings(); };
    window.addEventListener("focus", refresh);
    return () => window.removeEventListener("focus", refresh);
  }, [enabled]);
  // 替えた先のリモートセッションの設定を読む。このマシンの設定は読み直さない。選んだことを書いている途中で読むと、
  // 選ぶ前の設定が返るため。
  useEffect(() => {
    if (enabled && runTarget !== null) void settingsOf(runTarget).refresh();
  }, [enabled, runTarget]);
  usePolling(refreshSettings, { intervalMs: settingsPollIntervalMs, enabled, whileHidden: true });
  const thisMachine = useThisMachineSettings();
  const target = settingsOf(runTarget);
  const shown = useSyncExternalStore(target.subscribe, target.snapshot);
  const readOnce = useRef(false);
  if (isRead(thisMachine) && isRead(shown)) readOnce.current = true;
  return readOnce.current;
}

// updatePreset changes one preset's settings. The preset's id and name are changed with updatePresets.
export function updatePreset(id: string, change: (preset: SettingsPreset) => SettingsPreset): Promise<void> {
  return shownSettings().changePart({
    read: (document) => document.presets.find((preset) => preset.id === id),
    place: (document, preset) => ({ ...document, presets: document.presets.map((candidate) => candidate.id === id ? preset : candidate) }),
    path: `/settings/presets/${encodeURIComponent(id)}`,
    body: (base, preset) => ({ base, preset }),
  }, change);
}

// updatePresetInUse changes the settings of the preset this machine uses.
export function updatePresetInUse(change: (preset: SettingsPreset) => SettingsPreset): Promise<void> {
  return updatePreset(presetInUse(currentSettings()).id, change);
}

// updatePresets replaces the list of presets: adding, renaming, removing or reordering them.
export function updatePresets(change: (presets: SettingsPreset[]) => SettingsPreset[]): Promise<void> {
  return shownSettings().changePart({
    read: (document) => document.presets,
    place: (document, presets) => ({
      ...document,
      presets,
      presetInUse: presets.some((preset) => preset.id === document.presetInUse) ? document.presetInUse : standardPresetId,
    }),
    path: "/settings/presets",
    body: (base, presets) => ({ base, presets }),
  }, change);
}

export function updateGlobalSettings(change: (settings: GlobalSettings) => GlobalSettings): Promise<void> {
  return shownSettings().changePart({
    read: (document) => document.global,
    place: (document, settings) => ({ ...document, global: settings }),
    path: "/settings/global",
    body: (base, settings) => ({ base, settings }),
  }, change);
}

// updateMachine changes a machine's settings: its name, the preset it uses and its port.
export function updateMachine(id: string, change: (settings: MachineSettings) => MachineSettings): Promise<void> {
  return shownSettings().changePart({
    read: (document) => document.machines.find((candidate) => candidate.id === id)?.settings,
    place: (document, settings) => ({
      ...document,
      presetInUse: id === document.machineId
        ? (document.presets.some((preset) => preset.id === settings.presetId) ? settings.presetId ?? standardPresetId : standardPresetId)
        : document.presetInUse,
      machines: document.machines.map((candidate) => candidate.id === id ? { ...candidate, settings } : candidate),
    }),
    path: `/settings/machines/${encodeURIComponent(id)}`,
    body: (base, settings) => ({ base, settings }),
  }, change);
}

// removeMachine takes a machine that is no longer used off the list.
export function removeMachine(id: string): Promise<void> {
  return shownSettings().removeMachine(id);
}

// setInterfaceState は、このマシンの画面の状態を1つ残す（空の値は消す）。どのリモートセッションを選んでいても、
// このマシンのエンジンに書く。
export function setInterfaceState(key: InterfaceStateKey, value: string): Promise<void> {
  return thisMachineSettings.setInterfaceState(key, value);
}

// setTransfersStopped は、画面が相手にするエンジンの転送の待ち行列が、待っている転送を始めるかを決める。転送は、
// 選んでいるリモートセッションのエンジンが行う。
export function setTransfersStopped(stopped: boolean): Promise<void> {
  return shownSettings().setTransfersStopped(stopped);
}

// interfaceState は、このマシンの画面の状態を1つ読む。何も残していなければ ""。document は、このマシンの設定
// （useThisMachineSettings、currentThisMachineSettings）を渡す。
export function interfaceState(document: SettingsDocument | null, key: InterfaceStateKey): string {
  return document?.state.interface?.[key] ?? "";
}

// resetSettingsForTests forgets the documents, for tests that start each case from nothing. The screens are drawn
// with this machine's settings again.
export function resetSettingsForTests(document: SettingsDocument | null = null): void {
  remoteSettings.clear();
  shownRunTarget = null;
  thisMachineSettings.reset(document);
  shownListeners.forEach((listener) => listener());
}
