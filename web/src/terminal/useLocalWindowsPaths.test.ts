import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { aboutApi } from "../api/about";
import { terminalOverviewApi } from "../api/terminalOverview";
import { useLocalWindowsPaths } from "./useLocalWindowsPaths";

vi.mock("../api/about", () => ({ aboutApi: { about: vi.fn() } }));
const machineStatus = vi.fn();
vi.mock("../api/terminalOverview", () => ({ terminalOverviewApi: vi.fn(() => ({ machineStatus })) }));

beforeEach(() => {
  vi.mocked(aboutApi.about).mockReset();
  machineStatus.mockReset();
  vi.mocked(terminalOverviewApi).mockClear();
});

describe("useLocalWindowsPaths", () => {
  it.each(["windows", "linux", "darwin"])("enables local paths only when the engine reports Windows (%s)", async (os) => {
    vi.mocked(aboutApi.about).mockResolvedValue({ os } as Awaited<ReturnType<typeof aboutApi.about>>);
    const { result } = renderHook(() => useLocalWindowsPaths("shell", undefined));
    expect(result.current).toBe(false);
    await waitFor(() => expect(aboutApi.about).toHaveBeenCalledOnce());
    await waitFor(() => expect(result.current).toBe(os === "windows"));
  });

  it("leaves paths disabled when the engine OS cannot be read", async () => {
    vi.mocked(aboutApi.about).mockRejectedValue(new Error("offline"));
    const { result } = renderHook(() => useLocalWindowsPaths("shell", undefined));
    await waitFor(() => expect(aboutApi.about).toHaveBeenCalledOnce());
    expect(result.current).toBe(false);
  });

  // リモートセッションのシェルのファイルは、そのシェルを動かす sshc エンジンが読む。About は取り次がないので、
  // そのエンジンのマシンの状態から OS を読む。
  it.each(["windows", "linux"])("goes by the OS of the run target's engine for a shell on it (%s)", async (os) => {
    machineStatus.mockResolvedValue({ hostname: "build-box", os });
    const { result } = renderHook(() => useLocalWindowsPaths("shell", "build-box"));
    await waitFor(() => expect(machineStatus).toHaveBeenCalledOnce());
    expect(terminalOverviewApi).toHaveBeenCalledWith("build-box");
    await waitFor(() => expect(result.current).toBe(os === "windows"));
    expect(aboutApi.about).not.toHaveBeenCalled();
  });

  it("does not enable Windows paths for SSH on a run target", () => {
    const { result } = renderHook(() => useLocalWindowsPaths("ssh", "build-box"));
    expect(result.current).toBe(false);
    expect(machineStatus).not.toHaveBeenCalled();
  });

  it("does not enable Windows paths for SSH on the local engine", () => {
    const { result } = renderHook(() => useLocalWindowsPaths("ssh", undefined));
    expect(result.current).toBe(false);
    expect(aboutApi.about).not.toHaveBeenCalled();
  });
});
