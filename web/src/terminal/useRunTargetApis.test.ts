import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "../api/client";
import { terminalSessionsApi } from "../api/terminalSessions";
import { useRunTargetApis } from "./useRunTargetApis";

// fetch that never answers on its own, like a host that does not, and reports being aborted as the browser does.
function fetchThatWaits() {
  const signals: { path: string; signal: AbortSignal | undefined }[] = [];
  const stub = vi.fn((path: string, init: RequestInit = {}) => {
    const signal = init.signal ?? undefined;
    signals.push({ path, signal });
    return new Promise<Response>((_, reject) => {
      const abort = () => reject(new DOMException("Aborted", "AbortError"));
      if (signal?.aborted) abort();
      signal?.addEventListener("abort", abort);
    });
  });
  vi.stubGlobal("fetch", stub);
  return signals;
}

const onRunTarget: { alias: string | null } = { alias: "r540" };

describe("useRunTargetApis", () => {
  beforeEach(() => { apiClient.setCSRF("token"); });
  afterEach(() => { vi.unstubAllGlobals(); apiClient.clear(); });

  it("uses this machine's own API while no run target is chosen", () => {
    const { result } = renderHook(() => useRunTargetApis(null, 0));
    expect(result.current.sessions).toBe(terminalSessionsApi);
  });

  it("ends the reads of the run target it leaves, without a failure to report", async () => {
    const requests = fetchThatWaits();
    const { result, rerender } = renderHook(({ alias }) => useRunTargetApis(alias, 0), { initialProps: onRunTarget });
    const reads = Promise.allSettled([
      result.current.sessions.terminalSessions(),
      result.current.workspaces.list(),
      result.current.restore.offer(),
      result.current.sessions.shellProfiles(),
    ]);
    rerender({ alias: null });
    for (const read of await reads) expect(read).toMatchObject({ status: "rejected", reason: { name: "AbortError" } });
    expect(requests.map((request) => request.path)).toContain("/api/v1/engines/r540/terminal/sessions");
    expect(result.current.sessions).toBe(terminalSessionsApi);
  });

  it("does not end a change: the engine may be carrying it out already", async () => {
    const requests = fetchThatWaits();
    const { result, rerender } = renderHook(({ alias }) => useRunTargetApis(alias, 0), { initialProps: onRunTarget });
    void result.current.sessions.openTerminalSession({ kind: "shell" });
    await vi.waitFor(() => expect(requests).toHaveLength(1));
    rerender({ alias: null });
    expect(requests[0]!.signal).toBeUndefined();
  });

  it("builds the API again when the run target's Vault is unlocked", () => {
    const { result, rerender } = renderHook(({ revision }) => useRunTargetApis("r540", revision), { initialProps: { revision: 0 } });
    const before = result.current.sessions;
    rerender({ revision: 1 });
    expect(result.current.sessions).not.toBe(before);
  });
});
