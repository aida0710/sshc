import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { TerminalSession } from "../../api/terminalSessions";
import type { TerminalSessionsState } from "../sessions";
import { useTerminalWorkspaceController } from "../useTerminalWorkspaceController";
import { paneSessionIds } from "./layout";

const sessions: TerminalSession[] = ["one", "two", "three", "four", "five"].map((id) => ({
  id, title: id, kind: "shell", workspaceId: id === "three" ? "other" : "default", state: "connected", problem: "", startedAt: "2026-10-23T00:00:00Z",
}));
const workspaces = [{ id: "default", name: "", hidden: false, builtIn: true }];
const shells = { shellProfiles: vi.fn().mockResolvedValue({ profiles: [] }) };
const refresh = vi.fn().mockResolvedValue(undefined);

function setup() {
  return renderHook(({ listed, runTarget }: { listed: TerminalSession[]; runTarget: string | null }) => useTerminalWorkspaceController({
    shells, terminalSessions: { sessions: listed, refresh } as unknown as TerminalSessionsState, workspaces,
    enabled: false, section: "Terminal", navigate: vi.fn(), closeNavigation: vi.fn(), runTarget,
  }), { initialProps: { listed: sessions, runTarget: null as string | null } });
}

describe("split view session lifecycle", () => {
  it("focuses sidebar selections inside the split and preserves it when selecting another session", () => {
    const { result } = setup();
    act(() => result.current.splitView.split({ sourceSessionId: "two", targetSessionId: "one", edge: "right" }));
    act(() => result.current.showSession("one"));
    expect(result.current.activeSessionId).toBe("one");
    expect(paneSessionIds(result.current.splitView.state.root)).toEqual(["one", "two"]);
    act(() => result.current.showSession("three"));
    expect(paneSessionIds(result.current.splitView.state.root)).toEqual(["three"]);
    expect(result.current.splitView.groups).toHaveLength(1);
    act(() => result.current.splitView.dispatch({ type: "select-group", number: 1 }));
    expect(paneSessionIds(result.current.splitView.state.root)).toEqual(["one", "two"]);
  });
  it("takes a session out of a split that is not the one shown, and keeps the session", () => {
    const { result } = setup();
    act(() => result.current.splitView.split({ sourceSessionId: "two", targetSessionId: "one", edge: "right" }));
    act(() => result.current.splitView.split({ sourceSessionId: "three", targetSessionId: "two", edge: "bottom" }));
    act(() => result.current.showSession("four"));
    expect(paneSessionIds(result.current.splitView.groups[0]?.view.root ?? null)).toEqual(["one", "two", "three"]);

    act(() => result.current.splitView.removeFromSplit("two"));

    expect(paneSessionIds(result.current.splitView.groups[0]?.view.root ?? null)).toEqual(["one", "three"]);
    expect(result.current.splitView.groupBySession.has("two")).toBe(false);
    expect(paneSessionIds(result.current.splitView.state.root)).toEqual(["four"]);
  });
  it.each(["deleted", "exited"])("keeps surviving panes when the active session is %s", (kind) => {
    const { result, rerender } = setup();
    act(() => result.current.splitView.split({ sourceSessionId: "two", targetSessionId: "one", edge: "right" }));
    act(() => result.current.splitView.split({ sourceSessionId: "three", targetSessionId: "two", edge: "bottom" }));
    const listed = kind === "deleted" ? sessions.slice(0, 2) : sessions.map((session) => session.id === "three" ? { ...session, state: "exited" as const } : session);
    rerender({ listed, runTarget: null });
    expect(paneSessionIds(result.current.splitView.state.root)).toEqual(["one", "two"]);
    expect(result.current.activeSessionId).toBe("one");
  });
  it("discards the split when changing run target even if session IDs are identical", () => {
    const { result, rerender } = setup();
    act(() => result.current.splitView.split({ sourceSessionId: "two", targetSessionId: "one", edge: "right" }));
    act(() => result.current.showSession("four"));
    act(() => result.current.splitView.split({ sourceSessionId: "five", targetSessionId: "four", edge: "bottom" }));
    expect(result.current.splitView.groups).toHaveLength(2);
    rerender({ listed: sessions, runTarget: "remote" });
    expect(result.current.splitView.groups).toEqual([]);
    expect(paneSessionIds(result.current.splitView.state.root)).toHaveLength(1);
    rerender({ listed: sessions, runTarget: null });
    expect(result.current.splitView.groups).toEqual([]);
    // The session last focused on this machine (the pane the last split added) comes back, on its own.
    expect(paneSessionIds(result.current.splitView.state.root)).toEqual(["five"]);
    act(() => result.current.splitView.split({ sourceSessionId: "four", targetSessionId: "five", edge: "right" }));
    expect(result.current.splitView.groups[0]!.number).toBe(1);
  });
  it("prunes ended sessions while their group is not displayed", () => {
    const { result, rerender } = setup();
    act(() => result.current.splitView.split({ sourceSessionId: "two", targetSessionId: "one", edge: "right" }));
    act(() => result.current.showSession("three"));
    rerender({ listed: sessions.map((session) => session.id === "two" ? { ...session, exited: { code: 0, signal: "", at: "2026-10-23T01:00:00Z" } } : session), runTarget: null });
    expect(result.current.splitView.groups).toEqual([]);
    expect(result.current.activeSessionId).toBe("three");
  });
  it("permits sessions from another workspace but refuses ended sessions", () => {
    const { result, rerender } = setup();
    expect(result.current.splitView.canSplit("three")).toBe(true);
    rerender({ listed: sessions.map((session) => session.id === "three" ? { ...session, state: "exited" } : session), runTarget: null });
    expect(result.current.splitView.canSplit("three")).toBe(false);
    act(() => result.current.splitView.split({ sourceSessionId: "three", targetSessionId: "one", edge: "right" }));
    expect(paneSessionIds(result.current.splitView.state.root)).toEqual(["one"]);
  });
});
