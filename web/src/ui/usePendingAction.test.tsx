import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { usePendingAction } from "./usePendingAction";

describe("usePendingAction", () => {
  it("names the action being waited for until it settles", async () => {
    let finish: () => void = () => undefined;
    const { result } = renderHook(() => usePendingAction<"save" | "delete">());
    let started: Promise<void> = Promise.resolve();
    act(() => { started = result.current.start("save", () => new Promise<void>((resolve) => { finish = resolve; })); });
    expect(result.current.running).toBe("save");

    await act(async () => { finish(); await started; });
    expect(result.current.running).toBeNull();
  });

  it("does not start a second action while one is running", async () => {
    let finish: () => void = () => undefined;
    const second = vi.fn(async () => undefined);
    const { result } = renderHook(() => usePendingAction<"save" | "delete">());
    let started: Promise<void> = Promise.resolve();
    act(() => {
      started = result.current.start("save", () => new Promise<void>((resolve) => { finish = resolve; }));
      void result.current.start("delete", second);
    });
    expect(second).not.toHaveBeenCalled();
    expect(result.current.running).toBe("save");
    await act(async () => { finish(); await started; });
  });

  it("hands a failure back to the caller and stops waiting", async () => {
    const { result } = renderHook(() => usePendingAction<"save">());
    await act(async () => {
      await expect(result.current.start("save", () => Promise.reject(new Error("refused")))).rejects.toThrow("refused");
    });
    expect(result.current.running).toBeNull();
  });
});
