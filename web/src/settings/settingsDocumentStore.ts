import { apiClient, failureCode } from "../api/client";
import { putJSON, sendJSON } from "../api/guards";
import type { components } from "../api/schema";
import { validateOpenAPISchema } from "../api/validators.generated";

// 1つの sshc エンジンが持つ設定（GET <base>/settings）: 全体の設定、プリセット、同期するマシン、そのエンジンの
// マシンの状態。店は、このブラウザが知っている設定を持ち、変更を1つずつ順に書く。エンジンがまだ見ていない設定の
// 上に、変更を重ねて送らないためである。

export type SettingsDocument = components["schemas"]["SettingsDocument"];
export type GlobalSettings = components["schemas"]["GlobalSettings"];
export type SettingsPreset = components["schemas"]["SettingsPreset"];
export type MachineSettings = components["schemas"]["MachineSettings"];
export type InterfaceSettings = components["schemas"]["InterfaceSettings"];
export type InterfaceState = components["schemas"]["InterfaceState"];
export type InterfaceStateKey = keyof InterfaceState;

// standardPresetId is the preset every machine uses until it chooses another (application.DefaultPresetID).
export const standardPresetId = "default";

// settingsChangedCode is the engine's answer when what a change was based on moved underneath it.
const settingsChangedCode = "settings_changed";

export type SettingsState = {
  document: SettingsDocument | null;
  // loadFailed is set when the first read failed; the screens then run on the defaults.
  loadFailed: boolean;
};

// SettingsPart is one part of the document a change is written to: the global settings, the presets, one preset
// or one machine's settings.
export type SettingsPart<T> = {
  // read is the part as this browser knows it, or undefined when it is gone and nothing is to be sent.
  read: (document: SettingsDocument) => T | undefined;
  // place is the document with next in place of the part.
  place: (document: SettingsDocument, next: T) => SettingsDocument;
  // path は、その部分を書く先（エンジンの API の下の /settings/global など）。body は送る本文で、新しい値と、それを
  // 作った元の値を入れる。元の値がエンジンの側で変わっていれば、エンジンは settingsChangedCode で断る。
  path: string;
  body: (base: T, next: T) => unknown;
};

// SettingsDocumentStore は、1つの sshc エンジンの設定である。読むのも書くのも、そのエンジンへ送る。
export type SettingsDocumentStore = {
  snapshot(): SettingsState;
  subscribe(listener: () => void): () => void;
  // refresh は設定を読み直す。ここで変更する前に始めた読み取りの答えは捨てる。
  refresh(): Promise<void>;
  // changePart は、部分の写しから次の値を作り、前の変更のあとに書く。変わらなければ何も送らない。
  changePart<T>(part: SettingsPart<T>, change: (current: T) => T): Promise<void>;
  // removeMachine は、使わなくなったマシンを一覧から外す。
  removeMachine(id: string): Promise<void>;
  // setInterfaceState は、画面の並びを1つ残す（空の値は消す）。画面にはすぐ出し、エンジンはほかの値と一緒に少し
  // あとで書く。押した操作を待たせないためである。
  setInterfaceState(key: InterfaceStateKey, value: string): Promise<void>;
  // setTransfersStopped は、そのエンジンの転送の待ち行列が、待っている転送を始めるかを決める。エンジンはすぐ書いて
  // 待ち行列に効かせ、次の一覧で答える。
  setTransfersStopped(stopped: boolean): Promise<void>;
  // reset は設定を忘れる。毎回何も無い状態から始めるテストのためのもの。
  reset(document: SettingsDocument | null): void;
};

function sameJSON(first: unknown, second: unknown): boolean {
  return JSON.stringify(first) === JSON.stringify(second);
}

// createSettingsDocumentStore は、base（engineApiBase）のエンジンの設定を持つ店を作る。
export function createSettingsDocumentStore(base: string): SettingsDocumentStore {
  let state: SettingsState = { document: null, loadFailed: false };
  const listeners = new Set<() => void>();
  // writes chains every change, so each one reads its base after the one before it has landed.
  let writes: Promise<unknown> = Promise.resolve();
  // revision counts the changes made here; a read that started before one of them is dropped, so it never
  // takes back what this browser just chose.
  let revision = 0;
  // unsettledWrites は、画面に先に出したが、エンジンがまだ書き終えていない変更の数。その間に読んだ設定は、その変更の
  // 前のものかもしれないので出さない。書き終えたら revision を進め、書いている間に始めた読み取りも捨てる。
  let unsettledWrites = 0;
  let refreshing = false;

  function publish(next: SettingsState) {
    state = next;
    listeners.forEach((listener) => listener());
  }

  async function readDocument(): Promise<SettingsDocument> {
    return validateOpenAPISchema<SettingsDocument>("SettingsDocument", await apiClient.read(`${base}/settings`));
  }

  async function refresh(): Promise<void> {
    if (refreshing) return;
    refreshing = true;
    const startedRevision = revision;
    try {
      const document = await readDocument();
      // An unchanged document is not published again, so the screens keep the objects they hold and draw nothing
      // anew every few seconds.
      if (startedRevision === revision && unsettledWrites === 0 && !sameJSON(document, state.document)) publish({ ...state, document, loadFailed: false });
    } catch {
      if (state.document === null) publish({ ...state, document: null, loadFailed: true });
    } finally {
      refreshing = false;
    }
  }

  function enqueue<T>(write: () => Promise<T>): Promise<T> {
    const next = writes.then(write, write);
    writes = next.catch(() => undefined);
    return next;
  }

  // settle は、画面に先に出した変更を send で書く。書き終えるまで、読んだ設定を出さない（unsettledWrites）。
  async function settle<T>(send: () => Promise<T>): Promise<T> {
    unsettledWrites += 1;
    try {
      return await send();
    } finally {
      unsettledWrites -= 1;
      revision += 1;
    }
  }

  // writeWithRetry shows the change at once, sends it, and when the engine says the base moved (another machine
  // or tab changed the same settings), reads the settings again and redoes the change once on top of them.
  async function writeWithRetry(change: (document: SettingsDocument) => { next: SettingsDocument; send: () => Promise<unknown> } | null) {
    for (let attempt = 0; attempt < 2; attempt += 1) {
      if (state.document === null) await refresh();
      const document = state.document;
      if (document === null) throw new Error("settings_unavailable");
      const planned = change(document);
      if (planned === null) return;
      revision += 1;
      publish({ ...state, document: planned.next });
      try {
        await settle(planned.send);
        return;
      } catch (error) {
        revision += 1;
        const document = await readDocument().catch(() => null);
        publish({ ...state, document: document ?? state.document });
        if (failureCode(error) !== settingsChangedCode || attempt > 0) throw error;
      }
    }
  }

  return {
    snapshot: () => state,
    subscribe(listener) {
      listeners.add(listener);
      return () => { listeners.delete(listener); };
    },
    refresh,
    changePart(part, change) {
      return enqueue(() => writeWithRetry((document) => {
        const current = part.read(document);
        if (current === undefined) return null;
        const next = change(structuredClone(current));
        if (sameJSON(current, next)) return null;
        return { next: part.place(document, next), send: () => putJSON(`${base}${part.path}`, part.body(current, next), [settingsChangedCode]) };
      }));
    },
    removeMachine(id) {
      return enqueue(async () => {
        await apiClient.mutate<unknown>(`${base}/settings/machines/${encodeURIComponent(id)}`, { method: "DELETE" });
        revision += 1;
        if (state.document !== null) {
          publish({ ...state, document: { ...state.document, machines: state.document.machines.filter((machine) => machine.id !== id) } });
        }
      });
    },
    setInterfaceState(key, value) {
      const document = state.document;
      if (document === null || (document.state.interface?.[key] ?? "") === value) return Promise.resolve();
      const next = { ...document.state.interface };
      if (value === "") delete next[key];
      else next[key] = value;
      revision += 1;
      publish({ ...state, document: { ...document, state: { ...document.state, interface: next } } });
      const sent = settle(() => enqueue(() => sendJSON(`${base}/settings/state`, { method: "PUT", body: { interface: { [key]: value } } })));
      return sent.then(() => undefined);
    },
    setTransfersStopped(stopped) {
      return enqueue(async () => {
        await sendJSON(`${base}/settings/state`, { method: "PUT", body: { transfersStopped: stopped } });
        const document = state.document;
        if (document === null) return;
        revision += 1;
        publish({ ...state, document: { ...document, state: { ...document.state, transfersStopped: stopped } } });
      });
    },
    reset(document) {
      writes = Promise.resolve();
      revision += 1;
      publish({ document, loadFailed: false });
    },
  };
}
