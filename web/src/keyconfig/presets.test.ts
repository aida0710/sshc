import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { apiClient } from "../api/client";
import { defaultBindings, loadBindings } from "./bindings";
import { localStorageKeys } from "../ui/browserStorageKeys";
import { refreshPresets, savePresetBindings, selectPreset, updatePresets, type Preset } from "./presets";
let remote: Preset[];
beforeEach(async () => {
  localStorage.clear(); remote = [];
  vi.spyOn(apiClient, "read").mockImplementation(async () => ({ schemaVersion: 5, shortcutPresets: structuredClone(remote) }));
  vi.spyOn(apiClient, "mutate").mockImplementation(async (_path, options) => {
    const request = JSON.parse(options?.body as string) as { base: Preset[]; presets: Preset[] };
    if (JSON.stringify(request.base) !== JSON.stringify(remote)) throw new Error("conflict");
    remote = structuredClone(request.presets);
    return {};
  });
  await refreshPresets();
});
afterEach(() => { localStorage.clear(); vi.restoreAllMocks(); });
it("stores nothing while the browser only shows the default shortcuts", async () => {
  expect(localStorage.length).toBe(0);
  remote = [{ id: "work", name: "Work", bindings: { ...defaultBindings, home: ["Alt+H"] } }];
  await refreshPresets();
  expect(localStorage.length).toBe(0);
  expect(loadBindings()).toEqual(defaultBindings);
  selectPreset("work");
  expect(localStorage.getItem(localStorageKeys.shortcutPresetSelection)).toBe("work");
});
it("keeps selection local while applying remote edits and recovering from deletion", async () => {
  remote = [{ id: "work", name: "Work", bindings: { ...defaultBindings, home: ["Alt+H"] } }];
  await refreshPresets();
  expect(loadBindings()).toEqual(defaultBindings);
  selectPreset("work");
  expect(loadBindings().home).toEqual(["Alt+H"]);
  remote[0]!.bindings.home = ["Alt+J"];
  await refreshPresets();
  expect(loadBindings().home).toEqual(["Alt+J"]);
  remote = [];
  await refreshPresets();
  expect(localStorage.getItem(localStorageKeys.shortcutPresetSelection)).toBe("default");
  expect(loadBindings()).toEqual(defaultBindings);
});
it("rejects stale edits without overwriting remote presets or current bindings", async () => {
  await savePresetBindings({ ...defaultBindings, home: ["Alt+H"] });
  const snapshot = structuredClone(remote);
  remote[0]!.name = "Changed elsewhere";
  await expect(updatePresets(snapshot)).rejects.toThrow("conflict");
  expect(remote[0]!.name).toBe("Changed elsewhere");
  expect(loadBindings().home).toEqual(["Alt+H"]);
  await refreshPresets();
});
it("does not let an older refresh response undo a completed save", async () => {
  let finish!: (value: unknown) => void;
  vi.mocked(apiClient.read).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve; }));
  const pending = refreshPresets();
  await savePresetBindings({ ...defaultBindings, home: ["Alt+H"] });
  finish({ schemaVersion: 5, shortcutPresets: [] });
  await pending;
  expect(loadBindings().home).toEqual(["Alt+H"]);
  expect(localStorage.getItem(localStorageKeys.shortcutPresetSelection)).toBe(remote[0]!.id);
});
function refuseStoringBindings() {
  const store = Storage.prototype.setItem;
  return vi.spyOn(Storage.prototype, "setItem").mockImplementation(function (this: Storage, key: string, value: string) {
    if (key === localStorageKeys.shortcutBindings) throw new DOMException("The quota has been exceeded.", "QuotaExceededError");
    store.call(this, key, value);
  });
}
it("keeps presets editable when a background refresh cannot store the bindings", async () => {
  await savePresetBindings({ ...defaultBindings, home: ["Alt+H"] });
  remote[0]!.bindings.home = ["Alt+J"];
  const refusal = refuseStoringBindings();
  await refreshPresets();
  expect(loadBindings().home).toEqual(["Alt+H"]);
  refusal.mockRestore();
  await savePresetBindings({ ...defaultBindings, home: ["Alt+K"] });
  expect(loadBindings().home).toEqual(["Alt+K"]);
});
it("reports a refused storage when the user picks a preset and keeps the previous choice", async () => {
  remote = [{ id: "work", name: "Work", bindings: { ...defaultBindings, home: ["Alt+H"] } }];
  await refreshPresets();
  refuseStoringBindings();
  expect(() => selectPreset("work")).toThrow("quota");
  expect(localStorage.getItem(localStorageKeys.shortcutPresetSelection)).toBeNull();
  expect(loadBindings()).toEqual(defaultBindings);
});
