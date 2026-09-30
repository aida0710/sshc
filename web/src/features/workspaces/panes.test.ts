import { describe, expect, it } from "vitest";
import type { TerminalSession } from "../../api/terminalSessions";
import { paneIds, paneSessionIds, reduceLayout } from "./layout";
import { dockedPairLayout, paneForSession, withoutClosedSessions } from "./panes";

function session(id: string): TerminalSession {
  return {
    id,
    kind: "ssh",
    alias: id,
    title: id,
    startedAt: "2026-01-01T00:00:00Z",
    state: "connected",
    problem: "",
  };
}

describe("workspace panes", () => {
  it("makes a two-pane split when a session is docked on a terminal shown on its own", () => {
    const layout = dockedPairLayout(session("web"), session("db"), "right");

    expect(paneSessionIds(layout.root)).toEqual(["web", "db"]);
  });

  it("closes the panes of closed sessions, and gives up the split when one pane is left", () => {
    const pair = dockedPairLayout(session("web"), session("db"), "right");
    const three = reduceLayout(pair, {
      type: "dock-pane",
      targetPaneId: paneIds(pair.root)[0] ?? "",
      edge: "bottom",
      pane: paneForSession(session("cache")),
    });

    const withoutCache = withoutClosedSessions(three, new Set(["cache"]));
    expect(withoutCache === null ? null : paneSessionIds(withoutCache.root)).toEqual(["web", "db"]);
    expect(withoutClosedSessions(three, new Set(["db", "cache"]))).toBeNull();
  });

  it("keeps the pane of a session it is not told is closed, such as one opened a moment ago", () => {
    const pair = dockedPairLayout(session("web"), session("opened"), "right");

    expect(withoutClosedSessions(pair, new Set(["gone"]))).toBe(pair);
  });
});
