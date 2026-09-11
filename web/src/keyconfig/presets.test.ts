import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { apiClient } from "../api/client";
import { defaultBindings, loadBindings, storageKey } from "./bindings";
import { refreshPresets, savePresetBindings, selectPreset, selectionKey, updatePresets, type Preset } from "./presets";
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
it("migrates legacy bindings once and preserves the migration identity after a lost response", async () => {
  localStorage.removeItem(selectionKey);
  localStorage.setItem(storageKey, JSON.stringify({ ...defaultBindings, home: ["Alt+H"] }));
  const original = apiClient.mutate;
  vi.mocked(apiClient.mutate).mockImplementationOnce(async (_path, options) => {
    remote = (JSON.parse(options?.body as string) as { presets: Preset[] }).presets;
    throw new Error("response lost");
  });
  await refreshPresets();
  expect(localStorage.getItem(selectionKey)).toMatch(/^pending:/);
  expect(remote).toHaveLength(1);
  await refreshPresets();
  expect(remote).toHaveLength(1);
  expect(localStorage.getItem(selectionKey)).toBe(remote[0]!.id);
  expect(loadBindings().home).toEqual(["Alt+H"]);
  expect(original).toHaveBeenCalledTimes(1);
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
  expect(localStorage.getItem(selectionKey)).toBe("default");
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
  expect(localStorage.getItem(selectionKey)).toBe(remote[0]!.id);
});
