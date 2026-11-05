import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { useRunTargetLink, waitNoticeDelayMs } from "./useRunTargetLink";

function listing(connected: boolean) {
  return { list: vi.fn(async () => ({ engines: [{ alias: "r540", connected }] })) };
}

const refusal = new ApiError("remote_engine_unreachable", 502, null);

describe("useRunTargetLink", () => {
  beforeEach(() => { vi.useFakeTimers(); });
  afterEach(() => { vi.useRealTimers(); });

  async function waitLong() {
    await act(async () => { await vi.advanceTimersByTimeAsync(waitNoticeDelayMs); });
  }

  it("says nothing about a wait too short to notice", async () => {
    const { result } = renderHook(() => useRunTargetLink({ selected: "r540", api: listing(false), enabled: true, loaded: false, refusal: null }));
    await act(async () => { await vi.advanceTimersByTimeAsync(waitNoticeDelayMs - 1); });
    expect(result.current.phase).toBe("ready");
  });

  it("says a run target is being connected to while there is no connection", async () => {
    const { result } = renderHook(() => useRunTargetLink({ selected: "r540", api: listing(false), enabled: true, loaded: false, refusal: null }));
    await waitLong();
    expect(result.current.phase).toBe("connecting");
    expect(result.current.connectedEngines.has("r540")).toBe(false);
  });

  it("says the engine is awaited once the connection is there", async () => {
    const { result } = renderHook(() => useRunTargetLink({ selected: "r540", api: listing(true), enabled: true, loaded: false, refusal: null }));
    await waitLong();
    expect(result.current.phase).toBe("awaitingEngine");
  });

  it("moves on as soon as the connection comes up", async () => {
    const api = { list: vi.fn().mockResolvedValueOnce({ engines: [{ alias: "r540", connected: false }] }).mockResolvedValue({ engines: [{ alias: "r540", connected: true }] }) };
    const { result } = renderHook(() => useRunTargetLink({ selected: "r540", api, enabled: true, loaded: false, refusal: null }));
    await waitLong();
    expect(result.current.phase).toBe("connecting");
    await act(async () => { await vi.advanceTimersByTimeAsync(1_000); });
    expect(result.current.phase).toBe("awaitingEngine");
  });

  it("says this machine's sessions are being read, with no connection to wait for", async () => {
    const { result } = renderHook(() => useRunTargetLink({ selected: null, api: listing(false), enabled: false, loaded: false, refusal: null }));
    await waitLong();
    expect(result.current.phase).toBe("loading");
  });

  it("is ready once the engine answered", async () => {
    const { result } = renderHook(() => useRunTargetLink({ selected: "r540", api: listing(true), enabled: true, loaded: true, refusal: null }));
    await waitLong();
    expect(result.current.phase).toBe("ready");
  });

  it("reports a refusal at once, with its reason", async () => {
    const { result } = renderHook(() => useRunTargetLink({ selected: "r540", api: listing(false), enabled: true, loaded: true, refusal }));
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(result.current.phase).toBe("refused");
    expect(result.current.refusal).toBe(refusal);
  });

  it("does not ask about run targets while there are none", async () => {
    const api = listing(false);
    renderHook(() => useRunTargetLink({ selected: null, api, enabled: false, loaded: true, refusal: null }));
    await waitLong();
    expect(api.list).not.toHaveBeenCalled();
  });

  it("reads the list faster while waiting than once settled", async () => {
    const api = listing(true);
    const { rerender } = renderHook(({ loaded }) => useRunTargetLink({ selected: "r540", api, enabled: true, loaded, refusal: null }), { initialProps: { loaded: false } });
    await act(async () => { await vi.advanceTimersByTimeAsync(2_000); });
    const whileWaiting = api.list.mock.calls.length;
    expect(whileWaiting).toBeGreaterThanOrEqual(3);
    rerender({ loaded: true });
    api.list.mockClear();
    await act(async () => { await vi.advanceTimersByTimeAsync(2_000); });
    expect(api.list.mock.calls.length).toBeLessThanOrEqual(1);
  });
});
