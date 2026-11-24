import { sameContents } from "../ui/jsonContents";
import type { GlobalSettings, MachineSettings, SettingsDocument, SettingsPreset } from "./settingsStore";

// 設定の画面でまだ保存していない変更。宛先（全体の設定、プリセット、マシン）ごとに、項目の名前と「今の値から次の値を
// 作る関数」を持つ。値ではなく関数なので、編集している間にほかのマシンが同じ宛先のほかの項目を変えても、届いた値の上に
// 当て直せる（settingsStore の changePart が保存のときにするのと同じ）。同じ項目の新しい変更は、前の変更を置き換える。

type Change<T> = (current: T) => T;
type FieldChanges<T> = ReadonlyMap<string, Change<T>>;

export type SettingsEdits = {
  readonly global: FieldChanges<GlobalSettings>;
  readonly presets: ReadonlyMap<string, FieldChanges<SettingsPreset>>;
  readonly machines: ReadonlyMap<string, FieldChanges<MachineSettings>>;
};

export const noEdits: SettingsEdits = { global: new Map(), presets: new Map(), machines: new Map() };

// SettingsPartChange は、変更を1つの宛先へまとめたもの。保存は宛先ごとに1回書く。
export type SettingsPartChange =
  | { kind: "global"; change: Change<GlobalSettings> }
  | { kind: "preset"; id: string; change: Change<SettingsPreset> }
  | { kind: "machine"; id: string; change: Change<MachineSettings> };

// 新しい変更は最後に当てる。前にある同じ項目を消してから足すと、Map の順が当てる順になる。
function withChange<T>(changes: FieldChanges<T> | undefined, field: string, change: Change<T>): FieldChanges<T> {
  const next = new Map(changes);
  next.delete(field);
  return next.set(field, change);
}

export function editGlobal(edits: SettingsEdits, field: string, change: Change<GlobalSettings>): SettingsEdits {
  return { ...edits, global: withChange(edits.global, field, change) };
}

export function editPreset(edits: SettingsEdits, id: string, field: string, change: Change<SettingsPreset>): SettingsEdits {
  return { ...edits, presets: new Map(edits.presets).set(id, withChange(edits.presets.get(id), field, change)) };
}

export function editMachine(edits: SettingsEdits, id: string, field: string, change: Change<MachineSettings>): SettingsEdits {
  return { ...edits, machines: new Map(edits.machines).set(id, withChange(edits.machines.get(id), field, change)) };
}

function applyChanges<T>(changes: FieldChanges<T>, current: T): T {
  return [...changes.values()].reduce((value, change) => change(value), structuredClone(current));
}

// withEdits は、保存した設定に変更を当てた、画面に出す設定。どのプリセットを使うか（presetInUse）は、マシンの設定を
// 保存したときに変わるので、変更を当てても動かない。
export function withEdits(document: SettingsDocument, edits: SettingsEdits): SettingsDocument {
  return {
    ...document,
    global: applyChanges(edits.global, document.global),
    presets: document.presets.map((preset) => {
      const changes = edits.presets.get(preset.id);
      return changes === undefined ? preset : applyChanges(changes, preset);
    }),
    machines: document.machines.map((machine) => {
      const changes = edits.machines.get(machine.id);
      return changes === undefined ? machine : { ...machine, settings: applyChanges(changes, machine.settings) };
    }),
  };
}

// changedParts は、保存した設定と違うところのある宛先。元の値に戻した変更や、消えたプリセット・マシンの変更は数えない。
export function changedParts(document: SettingsDocument, edits: SettingsEdits): SettingsPartChange[] {
  const parts: SettingsPartChange[] = [];
  const differs = <T>(changes: FieldChanges<T>, current: T) => !sameContents(applyChanges(changes, current), current);
  if (differs(edits.global, document.global)) {
    parts.push({ kind: "global", change: (current) => applyChanges(edits.global, current) });
  }
  for (const [id, changes] of edits.presets) {
    const preset = document.presets.find((candidate) => candidate.id === id);
    if (preset !== undefined && differs(changes, preset)) parts.push({ kind: "preset", id, change: (current) => applyChanges(changes, current) });
  }
  for (const [id, changes] of edits.machines) {
    const machine = document.machines.find((candidate) => candidate.id === id);
    if (machine !== undefined && differs(changes, machine.settings)) parts.push({ kind: "machine", id, change: (current) => applyChanges(changes, current) });
  }
  return parts;
}

function withoutChanges<T>(changes: FieldChanges<T> | undefined, done: FieldChanges<T> | undefined): FieldChanges<T> {
  return new Map([...(changes ?? [])].filter(([field, change]) => done?.get(field) !== change));
}

// withoutPart は、保存し終えた宛先の変更を除く。saved は保存を始めたときの変更で、そのあとに足した変更は残す。
export function withoutPart(edits: SettingsEdits, saved: SettingsEdits, part: SettingsPartChange): SettingsEdits {
  if (part.kind === "global") return { ...edits, global: withoutChanges(edits.global, saved.global) };
  if (part.kind === "preset") {
    return { ...edits, presets: new Map(edits.presets).set(part.id, withoutChanges(edits.presets.get(part.id), saved.presets.get(part.id))) };
  }
  return { ...edits, machines: new Map(edits.machines).set(part.id, withoutChanges(edits.machines.get(part.id), saved.machines.get(part.id))) };
}
