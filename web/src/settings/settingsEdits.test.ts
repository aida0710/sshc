import { describe, expect, it } from "vitest";
import { settingsDocument, testMachineId } from "../testing/settingsServer";
import { changedParts, editGlobal, editMachine, editPreset, noEdits, withEdits, withoutPart } from "./settingsEdits";

const stored = settingsDocument({ presets: [{ id: "default" }, { id: "phone", name: "Phone", appearance: { theme: "dark" } }] });
const lightTheme = editPreset(noEdits, "phone", "theme", (preset) => ({ ...preset, appearance: { ...preset.appearance, theme: "light" } }));

describe("保存していない設定の変更", () => {
  it("変更を当てた設定を出し、保存した設定は変えない", () => {
    expect(withEdits(stored, lightTheme).presets[1]).toMatchObject({ id: "phone", appearance: { theme: "light" } });
    expect(stored.presets[1]).toMatchObject({ appearance: { theme: "dark" } });
  });

  it("編集中にほかのマシンが変えた項目の上に当て直す", () => {
    const elsewhere = { ...stored, presets: [{ id: "default" }, { id: "phone", name: "Phone", appearance: { theme: "dark", accent: "blue" } }] };
    expect(withEdits(elsewhere, lightTheme).presets[1]).toMatchObject({ appearance: { theme: "light", accent: "blue" } });
  });

  it("同じ項目を変え直すと、最後の変更だけが残る", () => {
    const twice = editPreset(lightTheme, "phone", "theme", (preset) => ({ ...preset, appearance: { ...preset.appearance, theme: "white-blue" } }));
    expect(withEdits(stored, twice).presets[1]).toMatchObject({ appearance: { theme: "white-blue" } });
    expect(twice.presets.get("phone")?.size).toBe(1);
  });

  it("元の値に戻した変更は、変わったところに数えない", () => {
    expect(changedParts(stored, lightTheme).map((part) => part.kind)).toEqual(["preset"]);
    const back = editPreset(lightTheme, "phone", "theme", (preset) => ({ ...preset, appearance: { ...preset.appearance, theme: "dark" } }));
    expect(changedParts(stored, back)).toEqual([]);
  });

  it("消して足し直して項目の順が変わっても、内容が同じなら変わったところに数えない", () => {
    const reordered = editPreset(noEdits, "phone", "name", ({ name, ...preset }) => ({ ...preset, name: name ?? "" }));
    expect(changedParts(stored, reordered)).toEqual([]);
  });

  it("編集中に消えたプリセットの変更は、変わったところに数えない", () => {
    expect(changedParts({ ...stored, presets: [{ id: "default" }] }, lightTheme)).toEqual([]);
  });

  it("使うプリセットをマシンの設定で変えても、保存するまで使っているプリセットは動かない", () => {
    const edits = editMachine(noEdits, testMachineId, "preset", (machine) => ({ ...machine, presetId: "phone" }));
    const drafted = withEdits(stored, edits);
    expect(drafted.machines[0]?.settings.presetId).toBe("phone");
    expect(drafted.presetInUse).toBe("default");
  });

  it("保存し終えた宛先の変更を除き、保存している間に足した変更は残す", () => {
    const both = editGlobal(lightTheme, "language", (settings) => ({ ...settings, language: "ja" }));
    const [globalPart, presetPart] = changedParts(stored, both);
    if (globalPart === undefined || presetPart === undefined) throw new Error("2つの宛先が変わっているはず");
    const editedWhileSaving = editPreset(both, "phone", "theme", (preset) => ({ ...preset, appearance: { theme: "white-blue" } }));

    const afterGlobal = withoutPart(editedWhileSaving, both, globalPart);
    expect(afterGlobal.global.size).toBe(0);
    expect(withEdits(stored, withoutPart(afterGlobal, both, presetPart)).presets[1]).toMatchObject({ appearance: { theme: "white-blue" } });
  });
});
