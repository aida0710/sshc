import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { noTerminalProgress } from "./osc9Progress";
import { useProgramProgress } from "./useProgramProgress";

describe("useProgramProgress", () => {
  it("shows what a sequence reports and says that it changed", () => {
    const changed = vi.fn();
    const { result } = renderHook(() => useProgramProgress(changed));

    act(() => result.current.apply(() => ({ state: "normal", percent: 40 })));

    expect(result.current.progress).toEqual({ state: "normal", percent: 40 });
    expect(changed).toHaveBeenCalledTimes(1);
  });

  it("says nothing when a program repeats the progress already shown", () => {
    const changed = vi.fn();
    const { result } = renderHook(() => useProgramProgress(changed));

    act(() => result.current.apply({ state: "indeterminate" }));
    act(() => result.current.apply({ state: "indeterminate" }));
    act(() => result.current.apply(() => ({ state: "normal", percent: 40 })));
    act(() => result.current.apply(() => ({ state: "normal", percent: 40 })));

    expect(changed).toHaveBeenCalledTimes(2);
  });

  it("forgets the progress when the view starts over, without saying so, since the engine clears its own", () => {
    const changed = vi.fn();
    const { result } = renderHook(() => useProgramProgress(changed));
    act(() => result.current.apply({ state: "normal", percent: 40 }));

    act(() => result.current.clear());

    expect(result.current.progress).toBe(noTerminalProgress);
    expect(changed).toHaveBeenCalledTimes(1);
  });
});
