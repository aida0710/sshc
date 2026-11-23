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
const refresh = vi.fn().mockResolvedValue(undefined);

function setup(maxSessions = 0) {
  return renderHook(({ listed, runTarget }: { listed: TerminalSession[]; runTarget: string | null }) => useTerminalWorkspaceController({
    terminalSessions: { sessions: listed, refresh, maxSessions } as unknown as TerminalSessionsState, workspaces,
    section: "Terminal", navigate: vi.fn(), closeNavigation: vi.fn(), runTarget,
  }), { initialProps: { listed: sessions, runTarget: null as string | null } });
}

describe("what can go beside a session", () => {
  it("can open a new session beside the only session, which has no other to go beside it", () => {
    const { result, rerender } = setup();
    rerender({ listed: sessions.slice(0, 1), runTarget: null });
    act(() => result.current.showSession("one"));
    expect(result.current.splitView.canSplit("one", "one")).toBe(false);
    expect(result.current.splitView.canOpenBeside("one")).toBe(true);
    expect(result.current.splitView.splitRefusal("one")).toBeNull();
  });

  it("says when the only session cannot have a new one beside it: at the session limit, or once it has ended", () => {
    const { result, rerender } = setup(1);
    rerender({ listed: sessions.slice(0, 1), runTarget: null });
    act(() => result.current.showSession("one"));
    expect(result.current.splitView.canOpenBeside("one")).toBe(false);
    expect(result.current.splitView.splitRefusal("one")).toBe("sessions");

    rerender({ listed: [{ ...sessions[0]!, state: "exited" }], runTarget: null });
    expect(result.current.splitView.splitRefusal("one")).toBe("ended");
  });

  it("does not offer a new pane in a split of four panes, and still lets the panes change places", () => {
    const { result } = setup();
    act(() => result.current.showSession("one"));
    for (const [source, target] of [["two", "one"], ["three", "two"], ["four", "three"]] as const) {
      act(() => result.current.splitView.split({ sourceSessionId: source, targetSessionId: target, edge: "right" }));
    }
    expect(result.current.splitView.canOpenBeside("one")).toBe(false);
    expect(result.current.splitView.canSplit("two", "one")).toBe(true);
    expect(result.current.splitView.splitRefusal("one")).toBeNull();
  });

  it("forgets a request for a split when the run target changes", () => {
    const { result, rerender } = setup();
    act(() => result.current.splitView.requestSplit({ targetSessionId: "one", edge: "right" }));
    expect(result.current.splitView.splitRequest).toEqual({ targetSessionId: "one", edge: "right" });
    rerender({ listed: sessions, runTarget: "bastion" });
    expect(result.current.splitView.splitRequest).toBeNull();
  });
});

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
  it("does not show one run target's splits on another, even if session IDs are identical", () => {
    const { result, rerender } = setup();
    act(() => result.current.splitView.split({ sourceSessionId: "two", targetSessionId: "one", edge: "right" }));
    act(() => result.current.showSession("four"));
    act(() => result.current.splitView.split({ sourceSessionId: "five", targetSessionId: "four", edge: "bottom" }));
    expect(result.current.splitView.groups).toHaveLength(2);
    rerender({ listed: sessions, runTarget: "remote" });
    expect(result.current.splitView.groups).toEqual([]);
    expect(paneSessionIds(result.current.splitView.state.root)).toHaveLength(1);
    rerender({ listed: sessions, runTarget: null });
    expect(result.current.splitView.groups.map((group) => paneSessionIds(group.view.root))).toEqual([["one", "two"], ["four", "five"]]);
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

describe("the splits of a run target", () => {
  function setupWithLoading() {
    return renderHook(({ listed, loaded, runTarget }: { listed: TerminalSession[]; loaded: boolean; runTarget: string | null }) => useTerminalWorkspaceController({
      terminalSessions: { sessions: listed, loaded, refresh, maxSessions: 0 } as unknown as TerminalSessionsState, workspaces,
      section: "Terminal", navigate: vi.fn(), closeNavigation: vi.fn(), runTarget,
    }), { initialProps: { listed: sessions, loaded: true, runTarget: null as string | null } });
  }
  const remoteSessions: TerminalSession[] = [{ ...sessions[0]!, id: "remote-one", title: "remote-one" }];

  it("are as they were left when the person comes back from another run target", () => {
    const { result, rerender } = setupWithLoading();
    act(() => result.current.showSession("one"));
    act(() => result.current.splitView.split({ sourceSessionId: "two", targetSessionId: "one", edge: "right" }));
    const leftRoot = result.current.splitView.state.root;

    // The other run target's list arrives after the change, and so does this machine's on the way back.
    rerender({ listed: [], loaded: false, runTarget: "bastion" });
    rerender({ listed: remoteSessions, loaded: true, runTarget: "bastion" });
    expect(result.current.splitView.groups).toEqual([]);
    expect(result.current.splitView.state.activeSessionId).toBe("remote-one");
    rerender({ listed: [], loaded: false, runTarget: null });
    expect(result.current.splitView.groups).toEqual([]);
    rerender({ listed: sessions, loaded: true, runTarget: null });

    expect(result.current.splitView.state.root).toEqual(leftRoot);
    expect(paneSessionIds(result.current.splitView.state.root)).toEqual(["one", "two"]);
    expect(result.current.splitView.state.activeSessionId).toBe("two");
  });

  it("lose the pane of a session that ended while another run target was shown", () => {
    const { result, rerender } = setupWithLoading();
    act(() => result.current.showSession("one"));
    act(() => result.current.splitView.split({ sourceSessionId: "two", targetSessionId: "one", edge: "right" }));
    act(() => result.current.splitView.split({ sourceSessionId: "four", targetSessionId: "two", edge: "right" }));

    rerender({ listed: remoteSessions, loaded: true, runTarget: "bastion" });
    rerender({ listed: sessions.filter((session) => session.id !== "two"), loaded: true, runTarget: null });

    expect(paneSessionIds(result.current.splitView.state.root)).toEqual(["one", "four"]);
  });
});
