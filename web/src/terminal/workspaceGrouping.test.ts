import { describe, expect, it } from "vitest";
import type { TerminalSession } from "../api/terminalSessions";
import type { TerminalWorkspace } from "../api/terminalWorkspaces";
import { beforeIdForWorkspaceMove, groupSessionsByWorkspace, sessionsAfterMove, workspaceDisplayName } from "./workspaceGrouping";

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

  it("puts a workspace moved before another right in front of it", () => {
    expect(beforeIdForWorkspaceMove(["default", "agents", "w1"], "w1", { targetId: "default", side: "before" })).toBe("default");
  });

  it("puts a workspace moved after another before the one that follows it, hidden or not", () => {
    expect(beforeIdForWorkspaceMove(["default", "agents", "w1"], "w1", { targetId: "default", side: "after" })).toBe("agents");
    expect(beforeIdForWorkspaceMove(["default", "agents", "w1"], "default", { targetId: "w1", side: "after" })).toBeNull();
    expect(beforeIdForWorkspaceMove(["w1", "default", "agents"], "w1", { targetId: "default", side: "after" })).toBe("agents");
  });
});

describe("sessionsAfterMove", () => {
  const all = [session("a1", "default"), session("b1", "w1"), session("a2", "default"), session("b2", "w1")];
  const placed = (sessions: readonly TerminalSession[]) => sessions.map((member) => `${member.id}@${member.workspaceId}`);

  it("puts a session moved before another right in front of it, in that session's workspace", () => {
    expect(placed(sessionsAfterMove(all, "a1", { workspaceId: "w1", beforeId: "b2" }))).toEqual(["b1@w1", "a2@default", "a1@w1", "b2@w1"]);
  });

  it("puts a session moved without a session to go before at the end, the last of its workspace", () => {
    expect(placed(sessionsAfterMove(all, "a1", { workspaceId: "default", beforeId: null }))).toEqual(["b1@w1", "a2@default", "b2@w1", "a1@default"]);
  });

  it("keeps the list as it is when the session to go before is gone, until the engine answers", () => {
    expect(placed(sessionsAfterMove(all, "a1", { workspaceId: "w1", beforeId: "closed" }))).toEqual(placed(all));
  });
});
