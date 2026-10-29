import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { useSeenProgramStatus } from "./useSeenProgramStatus";

function done(id: string, version: number): TerminalSession {
  return {
    id, kind: "shell", workspaceId: "default", title: id, startedAt: "2026-10-29T09:00:00Z", state: "connected", problem: "",
    programStatus: { state: "done", version },
  };
}

describe("useSeenProgramStatus", () => {
  afterEach(() => vi.restoreAllMocks());

  it("counts a result as seen on the session on screen, and only there", () => {
    vi.spyOn(document, "hasFocus").mockReturnValue(true);
    const { result, rerender } = renderHook(({ sessions, active }) => useSeenProgramStatus(sessions, active), {
      initialProps: { sessions: [done("a", 2), done("b", 5)], active: "a" },
    });
    expect(result.current.get("a")).toBe(2);
    expect(result.current.has("b")).toBe(false);

    rerender({ sessions: [done("a", 3), done("b", 5)], active: "a" });
    expect(result.current.get("a")).toBe(3);
    rerender({ sessions: [done("a", 3), done("b", 5)], active: "b" });
    expect(result.current.get("b")).toBe(5);
  });

  it("does not count a result that came while the person was away until they come back", () => {
    const focus = vi.spyOn(document, "hasFocus").mockReturnValue(false);
    const { result } = renderHook(() => useSeenProgramStatus([done("a", 2)], "a"));
    expect(result.current.has("a")).toBe(false);

    focus.mockReturnValue(true);
    act(() => { window.dispatchEvent(new Event("focus")); });
    expect(result.current.get("a")).toBe(2);
  });

  it("forgets a session once it is closed", () => {
    vi.spyOn(document, "hasFocus").mockReturnValue(true);
    const { result, rerender } = renderHook(({ sessions }) => useSeenProgramStatus(sessions, "a"), {
      initialProps: { sessions: [done("a", 2)] },
    });
    rerender({ sessions: [] });
    expect(result.current.size).toBe(0);
  });
});
