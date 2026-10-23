import { describe, expect, it } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import type { TerminalWorkspace } from "../api/terminalWorkspaces";
import { groupSessionsByWorkspace, orderWithGroupReordered, orderWithSessionMovedInto, workspaceDisplayName } from "./workspaceGrouping";

const workspaces: TerminalWorkspace[] = [
  { id: "default", name: "", hidden: false, builtIn: true },
  { id: "agents", name: "", hidden: true, builtIn: true },
  { id: "w1", name: "sshc開発", hidden: false, builtIn: false },
];

const session = (id: string, workspaceId: string): TerminalSession => ({
  id, kind: "shell", title: id, workspaceId, startedAt: "2026-10-22T00:00:00Z", state: "connected", problem: "",
});

describe("workspace grouping", () => {
  it("leaves hidden workspaces out unless asked for", () => {
    const sessions = [session("a", "default"), session("b", "agents"), session("c", "w1")];
    expect(groupSessionsByWorkspace(workspaces, sessions, false).map((group) => group.workspace.id)).toEqual(["default", "w1"]);
    const shown = groupSessionsByWorkspace(workspaces, sessions, true);
    expect(shown.map((group) => group.sessions.map((member) => member.id))).toEqual([["a"], ["b"], ["c"]]);
  });

  it("shows a session of a workspace it has not heard of under default", () => {
    const groups = groupSessionsByWorkspace(workspaces, [session("a", "made-elsewhere")], false);
    expect(groups[0]?.sessions.map((member) => member.id)).toEqual(["a"]);
  });

  it("names a built-in workspace in the screen's language until it is renamed", () => {
    const t = (key: string) => key;
    expect(workspaceDisplayName(t as never, workspaces[0]!)).toBe("workspace.defaultName");
    expect(workspaceDisplayName(t as never, workspaces[1]!)).toBe("workspace.agentsName");
    expect(workspaceDisplayName(t as never, workspaces[2]!)).toBe("sshc開発");
  });

  it("reorders one workspace without moving the sessions of another", () => {
    expect(orderWithGroupReordered(["a", "x", "b", "y", "c"], ["c", "a", "b"])).toEqual(["c", "x", "a", "y", "b"]);
  });
});

describe("orderWithSessionMovedInto", () => {
  const all = ["a1", "b1", "a2", "b2", "c1"];

  it("puts a session dropped on a session of the other workspace before it", () => {
    expect(orderWithSessionMovedInto(all, { sessionId: "c1", members: ["b1", "b2"], beforeId: "b2" })).toEqual(["a1", "b1", "a2", "c1", "b2"]);
  });

  it("puts a session dropped on the workspace after its last session", () => {
    expect(orderWithSessionMovedInto(all, { sessionId: "a1", members: ["b1", "b2"], beforeId: null })).toEqual(["b1", "a2", "b2", "a1", "c1"]);
  });

  it("keeps the place of a session dropped on an empty workspace", () => {
    expect(orderWithSessionMovedInto(all, { sessionId: "a2", members: [], beforeId: null })).toEqual(all);
  });
});
