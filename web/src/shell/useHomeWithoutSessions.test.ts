import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import { useHomeWithoutSessions } from "./useHomeWithoutSessions";

const session = { id: "one" } as TerminalSession;
const none = { sessions: [], loaded: true, opening: false, remoteRefusal: null };

function shownHome(terminalFace: boolean, terminalSessions: Parameters<typeof useHomeWithoutSessions>[0]["terminalSessions"]) {
  const showHome = vi.fn();
  renderHook(() => useHomeWithoutSessions({ terminalFace, terminalSessions, showHome }));
  return showHome.mock.calls.length > 0;
}

describe("useHomeWithoutSessions", () => {
  it("shows Home when the terminal is shown and the engine has no session", () => {
    expect(shownHome(true, none)).toBe(true);
  });

  it("leaves another section alone", () => {
    expect(shownHome(false, none)).toBe(false);
  });

  it("leaves the terminal on the screen while it has a session, waits for the engine, opens a session, or is refused", () => {
    expect(shownHome(true, { ...none, sessions: [session] })).toBe(false);
    expect(shownHome(true, { ...none, loaded: false })).toBe(false);
    expect(shownHome(true, { ...none, opening: true })).toBe(false);
    expect(shownHome(true, { ...none, remoteRefusal: { code: "vault_locked" } as never })).toBe(false);
  });

  it("shows Home once the last session is closed", () => {
    const showHome = vi.fn();
    const { rerender } = renderHook(
      ({ sessions }) => useHomeWithoutSessions({ terminalFace: true, terminalSessions: { ...none, sessions }, showHome }),
      { initialProps: { sessions: [session] } },
    );
    expect(showHome).not.toHaveBeenCalled();
    rerender({ sessions: [] });
    expect(showHome).toHaveBeenCalledOnce();
  });
});
