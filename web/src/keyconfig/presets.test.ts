import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { apiClient } from "../api/client";
import { presetInterfaceOf } from "../settings/presetInterface";
import { currentSettings } from "../settings/settingsStore";
import { fakeSettingsServer } from "../testing/settingsServer";
import { defaultBindings } from "./bindings";
import { currentBindings, refreshPresets, savePresetBindings, selectPreset, updatePresets, usePresets, type Preset } from "./presets";

let remote: Preset[];
let server: ReturnType<typeof fakeSettingsServer>;
beforeEach(async () => {
  remote = [];
  server = fakeSettingsServer({}, {
    read: async () => ({ schemaVersion: 10, presets: [{ id: "default" }], shortcutPresets: structuredClone(remote) }),
    mutate: async (_path, init) => {
      const request = JSON.parse(init?.body as string) as { base: Preset[]; presets: Preset[] };
      if (JSON.stringify(request.base) !== JSON.stringify(remote)) throw new Error("conflict");
      remote = structuredClone(request.presets);
      return {};
    },
  });
  await refreshPresets();
});
afterEach(() => vi.restoreAllMocks());

const work: Preset = { id: "work", name: "Work", bindings: { ...defaultBindings, home: ["Alt+H"] } };

it("keeps the default shortcuts until a shortcut preset is chosen, and keeps the choice in the preset in use", async () => {
  remote = [structuredClone(work)];
  await refreshPresets();
  expect(currentBindings()).toEqual(defaultBindings);

  await selectPreset("work");

  expect(currentBindings().home).toEqual(["Alt+H"]);
  expect(server.document().presets[0]?.interface?.shortcutPreset).toBe("work");
});

it("shows the defaults as chosen where the chosen shortcut preset was deleted", async () => {
  remote = [structuredClone(work)];
  await refreshPresets();
  await selectPreset("work");
  const { result } = renderHook(() => usePresets());
  expect(result.current.selected).toBe("work");

  remote = [];
  await act(async () => { await refreshPresets(); });

  // The settings preset still names "work"; what is in force, and shown, are the defaults.
  expect(result.current.selected).toBe("default");
  expect(currentBindings()).toEqual(defaultBindings);
});

it("follows remote edits and falls back to the defaults when the chosen preset is deleted", async () => {
  remote = [structuredClone(work)];
  await refreshPresets();
  await selectPreset("work");
  remote[0]!.bindings.home = ["Alt+J"];
  await refreshPresets();
  expect(currentBindings().home).toEqual(["Alt+J"]);

  remote = [];
  await refreshPresets();

  expect(currentBindings()).toEqual(defaultBindings);
});

it("rejects stale edits without overwriting remote presets or current bindings", async () => {
  await savePresetBindings({ ...defaultBindings, home: ["Alt+H"] });
  const snapshot = structuredClone(remote);
  remote[0]!.name = "Changed elsewhere";
  await expect(updatePresets(snapshot, snapshot[0]!.id)).rejects.toThrow("conflict");
  expect(remote[0]!.name).toBe("Changed elsewhere");
  expect(currentBindings().home).toEqual(["Alt+H"]);
  await refreshPresets();
});

it("does not let an older refresh response undo a completed save", async () => {
  let finish!: (value: unknown) => void;
  vi.mocked(apiClient.read).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve; }));
  const pending = refreshPresets();
  await savePresetBindings({ ...defaultBindings, home: ["Alt+H"] });
  finish({ schemaVersion: 10, presets: [{ id: "default" }], shortcutPresets: [] });
  await pending;
  expect(currentBindings().home).toEqual(["Alt+H"]);
  expect(presetInterfaceOf(currentSettings()).shortcutPreset).toBe(remote[0]!.id);
});
