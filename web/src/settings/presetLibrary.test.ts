import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { KeyBarPreset } from "../keybar/keyBarPresets";
import { fakeSettingsServer } from "../testing/settingsServer";
import { createPresetLibrary, refreshPresetLibraries, usePresetLibrarySync } from "./presetLibrary";

const mine: KeyBarPreset = { id: "mine", name: "My key order", keys: [{ key: "Escape" }], moreKeys: [] };
const metadata = { schemaVersion: 10, presets: [{ id: "default" }], keyBarPresets: [mine] };

let read: ReturnType<typeof vi.fn<() => Promise<unknown>>>;

beforeEach(() => {
  read = vi.fn(async () => structuredClone(metadata));
  fakeSettingsServer({}, { read: () => read() });
});
afterEach(() => vi.restoreAllMocks());

// makeLibrary は、画面のコードが読み込まれたときと同じように、一覧を作る。
const makeLibrary = () => createPresetLibrary<KeyBarPreset>({
  endpoint: "/api/v1/metadata/keybar", choice: "keyBarPreset", read: (document) => document.keyBarPresets,
});

it("reads a library made after the first read at once, without waiting for the next poll", async () => {
  const sync = renderHook(() => usePresetLibrarySync(true));
  await waitFor(() => expect(read).toHaveBeenCalledOnce());
  await act(async () => { await refreshPresetLibraries(); });

  const library = makeLibrary();
  const { result } = renderHook(() => library.usePresets());

  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.presets).toEqual([mine]);
  sync.unmount();
});

it("reads again for a library made while a read is under way", async () => {
  let answer: (value: unknown) => void = () => {};
  read.mockImplementationOnce(() => new Promise((resolve) => { answer = resolve; }));
  const sync = renderHook(() => usePresetLibrarySync(true));
  await waitFor(() => expect(read).toHaveBeenCalledOnce());

  const library = makeLibrary();
  const { result } = renderHook(() => library.usePresets());
  expect(read).toHaveBeenCalledOnce();
  await act(async () => { answer(structuredClone(metadata)); });

  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.presets).toEqual([mine]);
  expect(read).toHaveBeenCalledTimes(2);
  sync.unmount();
});

it("does not read for a library made while the vault is locked", async () => {
  const sync = renderHook(() => usePresetLibrarySync(true));
  await waitFor(() => expect(read).toHaveBeenCalledOnce());
  sync.unmount();
  read.mockClear();

  const library = makeLibrary();
  const { result } = renderHook(() => library.usePresets());
  await act(async () => { await Promise.resolve(); });

  expect(read).not.toHaveBeenCalled();
  expect(result.current.loading).toBe(true);
});
