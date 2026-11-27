import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "../api/client";
import { settingsDocument } from "../testing/settingsServer";
import {
  currentSettings,
  currentThisMachineSettings,
  interfaceState,
  refreshSettings,
  resetSettingsForTests,
  setInterfaceState,
  setTransfersStopped,
  updateGlobalSettings,
  useSettings,
  useSettingsSync,
  useThisMachineSettings,
} from "./settingsStore";

const thisMachineId = "11111111111111111111111111111111";
const remoteMachineId = "22222222222222222222222222222222";

// twoEngines は、このマシンのエンジンと、リモートセッション r100 のエンジンの設定を答える。書いた先の道筋を残す。
function twoEngines() {
  const thisMachine = settingsDocument({ machineId: thisMachineId, state: { facts: { os: "linux" }, interface: { runTarget: "r100" } } });
  const remote = settingsDocument({ machineId: remoteMachineId });
  const written: string[] = [];
  vi.spyOn(apiClient, "read").mockImplementation(async (path: string) => {
    if (path === "/api/v1/settings") return structuredClone(thisMachine);
    if (path === "/api/v1/engines/r100/settings") return structuredClone(remote);
    throw new Error(`unexpected read ${path}`);
  });
  vi.spyOn(apiClient, "mutate").mockImplementation(async (path: string) => {
    written.push(path);
    return { transactionId: "test", written: [] };
  });
  resetSettingsForTests(null);
  return { written };
}

afterEach(() => {
  vi.restoreAllMocks();
  resetSettingsForTests(null);
});

describe("settingsStore with a remote session chosen", () => {
  it("draws the screens with the remote engine's settings, and keeps the choice and the layout on this machine", async () => {
    const { written } = twoEngines();
    const { result } = renderHook(() => ({
      loaded: useSettingsSync(true, "r100"),
      shown: useSettings().document?.machineId,
      thisMachine: useThisMachineSettings().document?.machineId,
    }));
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.shown).toBe(remoteMachineId);
    expect(result.current.thisMachine).toBe(thisMachineId);

    await act(async () => {
      await updateGlobalSettings((settings) => ({ ...settings, updateNoticeFoldedRelease: "v9.9.9" }));
      await setTransfersStopped(true);
      await setInterfaceState("sftpPanes", "[]");
    });
    expect(written).toEqual([
      "/api/v1/engines/r100/settings/global",
      "/api/v1/engines/r100/settings/state",
      "/api/v1/settings/state",
    ]);
  });

  it("waits for the remote engine's settings before saying they are read", async () => {
    let answer: (document: unknown) => void = () => {};
    twoEngines();
    vi.mocked(apiClient.read).mockImplementation(async (path: string) => {
      if (path === "/api/v1/settings") return settingsDocument({ machineId: thisMachineId });
      return new Promise((resolve) => { answer = resolve; });
    });
    const { result } = renderHook(() => useSettingsSync(true, "r100"));
    await waitFor(() => expect(vi.mocked(apiClient.read)).toHaveBeenCalledWith("/api/v1/engines/r100/settings"));
    expect(result.current).toBe(false);
    await act(async () => { answer(settingsDocument({ machineId: remoteMachineId })); });
    await waitFor(() => expect(result.current).toBe(true));
  });

  it("draws with this machine's settings again once localhost is chosen, and the remote one's at once when it is chosen again", async () => {
    twoEngines();
    const initialProps: { runTarget: string | null } = { runTarget: "r100" };
    const { result, rerender } = renderHook(({ runTarget }: { runTarget: string | null }) => {
      useSettingsSync(true, runTarget);
      return useSettings().document?.machineId;
    }, { initialProps });
    await waitFor(() => expect(result.current).toBe(remoteMachineId));

    rerender({ runTarget: null });
    expect(result.current).toBe(thisMachineId);
    expect(currentSettings()?.machineId).toBe(thisMachineId);

    rerender({ runTarget: "r100" });
    expect(result.current).toBe(remoteMachineId);
  });

  it("does not take back a choice with a read that answers while the choice is still being written", async () => {
    twoEngines();
    let finishWrite: () => void = () => {};
    vi.mocked(apiClient.mutate).mockImplementation(() => new Promise((resolve) => { finishWrite = () => resolve({ transactionId: "test", written: [] }); }));
    await act(async () => { await refreshSettings(); });
    vi.mocked(apiClient.read).mockImplementation(async () => settingsDocument({ machineId: thisMachineId }));

    let written: Promise<void> = Promise.resolve();
    act(() => { written = setInterfaceState("runTarget", "r200"); });
    await act(async () => { await refreshSettings(); });
    expect(interfaceState(currentThisMachineSettings(), "runTarget")).toBe("r200");

    await act(async () => { finishWrite(); await written; });
    expect(interfaceState(currentThisMachineSettings(), "runTarget")).toBe("r200");
  });
});
