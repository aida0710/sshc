import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { aboutApi } from "../api/about";
import { useLocalWindowsPaths } from "./useLocalWindowsPaths";

vi.mock("../api/about", () => ({ aboutApi: { about: vi.fn() } }));

beforeEach(() => { vi.mocked(aboutApi.about).mockReset(); });

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

  it.each(["ssh", "shell"] as const)("does not enable Windows paths for a %s on a run target", (kind) => {
    const { result } = renderHook(() => useLocalWindowsPaths(kind, "build-box"));
    expect(result.current).toBe(false);
    expect(aboutApi.about).not.toHaveBeenCalled();
  });

  it("does not enable Windows paths for SSH on the local engine", () => {
    const { result } = renderHook(() => useLocalWindowsPaths("ssh", undefined));
    expect(result.current).toBe(false);
    expect(aboutApi.about).not.toHaveBeenCalled();
  });
});
