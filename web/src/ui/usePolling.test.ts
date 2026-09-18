import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { usePolling } from "./usePolling";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("usePolling", () => {
  it("ticks on the interval and once more at the start when asked", () => {
    const tick = vi.fn();
    renderHook(() => usePolling(tick, { intervalMs: 1_000, immediately: true }));
    expect(tick).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(2_000);
    expect(tick).toHaveBeenCalledTimes(3);
  });

  it("does not start the next tick while the previous one is still running", async () => {
    let finish = () => undefined as void;
    const tick = vi.fn(() => new Promise<void>((resolve) => { finish = resolve; }));
    renderHook(() => usePolling(tick, { intervalMs: 100 }));
    vi.advanceTimersByTime(350);
    expect(tick).toHaveBeenCalledTimes(1);
    finish();
    await vi.advanceTimersByTimeAsync(100);
    expect(tick).toHaveBeenCalledTimes(2);
  });

  it("skips ticks while the page is hidden unless told to continue", () => {
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
    const paused = vi.fn();
    const background = vi.fn();
    renderHook(() => usePolling(paused, { intervalMs: 100 }));
    renderHook(() => usePolling(background, { intervalMs: 100, whileHidden: true }));
    vi.advanceTimersByTime(250);
    expect(paused).not.toHaveBeenCalled();
    expect(background).toHaveBeenCalledTimes(2);
  });

  it("stops when disabled and calls the latest closure when it runs", () => {
    const first = vi.fn();
    const second = vi.fn();
    const { rerender } = renderHook(({ tick, enabled }) => usePolling(tick, { intervalMs: 100, enabled }), {
      initialProps: { tick: first, enabled: true },
    });
    rerender({ tick: second, enabled: true });
    vi.advanceTimersByTime(100);
    expect(first).not.toHaveBeenCalled();
    expect(second).toHaveBeenCalledTimes(1);
    rerender({ tick: second, enabled: false });
    vi.advanceTimersByTime(500);
    expect(second).toHaveBeenCalledTimes(1);
  });
});
