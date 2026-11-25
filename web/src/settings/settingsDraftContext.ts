import { createContext, useContext, useEffect, useId, useRef } from "react";
import type { GlobalSettings, InterfaceSettings, MachineSettings, SettingsDocument, SettingsPreset } from "./settingsStore";
import { withInterface, type InterfaceChange } from "./presetInterface";

// 設定の画面の下書き。どのページも、変更をここに置き、［保存］でまとめて適用する（利用者が選んだ形、2026-11-25）。

// DraftPart は、設定とは別の宛先に保存する下書き（ショートカットとキーバーの並び）。
export type DraftPart = {
  dirty: boolean;
  save: () => Promise<void>;
  discard: () => void;
};

// DraftProblem は、まだ保存できない理由。outOfSight は、その欄が画面に出ていないとき。
export type DraftProblem = { text: string; outOfSight: boolean };

// DescribeFailure は、保存できなかった理由を文にする。自分の項目の失敗でなければ null を返す。その項目を見える所に
// 出す（Terminal のタブを開く）のもここでする。
export type DescribeFailure = (error: unknown) => string | null;

export type SettingsDraft = {
  // document は、保存していない変更を当てた設定。loadFailed は、最初に読めなかったとき。
  document: SettingsDocument | null;
  loadFailed: boolean;
  // field は項目の名前。同じ名前の新しい変更は、前の変更を置き換える。
  editGlobal: (field: string, change: (settings: GlobalSettings) => GlobalSettings) => void;
  editPreset: (id: string, field: string, change: (preset: SettingsPreset) => SettingsPreset) => void;
  editMachine: (id: string, field: string, change: (settings: MachineSettings) => MachineSettings) => void;
  // discarded は、破棄した回数。打つ欄は、これが変わったら手元の文字を捨てる。
  discarded: number;
  // confirmBeforeDiscarding は、保存していない変更があれば破棄してよいかを確かめてから action を行う。
  confirmBeforeDiscarding: (action: () => void) => void;
  dirty: boolean;
  // inputInvalid は、直すまで保存できない値（書きかけの値など）があるか。problemOutOfSight は、そのうち今の画面に欄が
  // 見えないものの理由。failure は、保存できなかった理由。文は、無ければ ""。
  inputInvalid: boolean;
  problemOutOfSight: string;
  failure: string;
  saving: boolean;
  // savedCount は、保存し終えた回数。
  savedCount: number;
  save: () => void;
  discard: () => void;
  setProblem: (id: string, problem: DraftProblem | null) => void;
  setPart: (id: string, part: DraftPart | null) => void;
  addDescribeFailure: (describe: DescribeFailure) => () => void;
};

export const SettingsDraftContext = createContext<SettingsDraft | null>(null);

export function useSettingsDraft(): SettingsDraft {
  const draft = useContext(SettingsDraftContext);
  if (draft === null) throw new Error("設定の項目は SettingsDraftProvider の中に置く");
  return draft;
}

// usePresetInterfaceEdit は、プリセットの画面の設定（InterfaceSettings）の下書きを変える関数を返す。
export function usePresetInterfaceEdit(presetId: string) {
  const { editPreset } = useSettingsDraft();
  return (field: keyof InterfaceSettings, change: (settings: InterfaceSettings) => InterfaceChange) =>
    editPreset(presetId, `interface.${field}`, (preset) => withInterface(preset, change));
}

// useDraftProblem は、書きかけの値など、まだ保存できない理由を伝える。理由があるあいだ［保存］を止める。理由は欄の下に
// 出ているので、帯では繰り返さない。欄が見えていないとき（Terminal のほかのタブ）は outOfSight で、帯に出す。
export function useDraftProblem(problem: string, { outOfSight = false }: { outOfSight?: boolean } = {}): void {
  const id = useId();
  const { setProblem } = useSettingsDraft();
  useEffect(() => {
    setProblem(id, problem === "" ? null : { text: problem, outOfSight });
    return () => setProblem(id, null);
  }, [id, outOfSight, problem, setProblem]);
}

// useDraftPart は、設定とは別の宛先に保存する下書きを伝える。
export function useDraftPart(part: DraftPart): void {
  const id = useId();
  const { setPart } = useSettingsDraft();
  const latest = useRef(part);
  latest.current = part;
  const { dirty } = part;
  useEffect(() => {
    setPart(id, { dirty, save: () => latest.current.save(), discard: () => latest.current.discard() });
    return () => setPart(id, null);
  }, [dirty, id, setPart]);
}

// useDescribeFailure は、保存できなかった理由のうち、自分の項目のものを文にする関数を伝える。
export function useDescribeFailure(describe: DescribeFailure): void {
  const { addDescribeFailure } = useSettingsDraft();
  const latest = useRef(describe);
  latest.current = describe;
  useEffect(() => addDescribeFailure((error) => latest.current(error)), [addDescribeFailure]);
}
