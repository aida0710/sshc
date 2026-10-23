import { describe, expect, it } from "vitest";
import { paneSessionIds } from "./layout";
import { activeSplitView, emptySplitGroups, maximumSplitGroups, reduceSplitGroups, type SplitGroupsState } from "./splitGroups";

function createGroup(state: SplitGroupsState, targetSessionId: string, sourceSessionId: string): SplitGroupsState {
  const selected = reduceSplitGroups(state, { type: "select", sessionId: targetSessionId });
  return reduceSplitGroups(selected, { type: "split", targetSessionId, sourceSessionId, edge: "right" });
}
const pair = createGroup(emptySplitGroups, "build", "logs");
const twoGroups = createGroup(pair, "tests", "monitor");
const members = (state: SplitGroupsState) => state.groups.map((group) => paneSessionIds(group.view.root));

describe("split groups", () => {
  it("creates independent groups and returns to their layout after selecting a standalone session", () => {
    const resized = reduceSplitGroups(twoGroups, { type: "resize", path: [], ratio: 65 });
    const single = reduceSplitGroups(resized, { type: "select", sessionId: "spare" });
    expect(single.activeGroupNumber).toBeNull();
    expect(paneSessionIds(activeSplitView(single).root)).toEqual(["spare"]);
    expect(members(single)).toEqual([["build", "logs"], ["tests", "monitor"]]);
    const restored = reduceSplitGroups(single, { type: "select-group", number: 2 });
    expect(activeSplitView(restored).root).toMatchObject({ ratio: 65 });
    expect(activeSplitView(restored).activeSessionId).toBe("monitor");
    expect(reduceSplitGroups(restored, { type: "select-group", number: 99 })).toBe(restored);
  });
  it("selects a grouped session and focuses its pane without changing the layout", () => {
    const selected = reduceSplitGroups(twoGroups, { type: "select", sessionId: "build" });
    expect(selected.activeGroupNumber).toBe(1);
    expect(activeSplitView(selected).activeSessionId).toBe("build");
    expect(activeSplitView(selected).root).toBe(pair.groups[0]!.view.root);
  });
  it("adds to the displayed group and refuses a fifth pane without changing membership", () => {
    let state = twoGroups;
    for (const sourceSessionId of ["three", "four"]) state = reduceSplitGroups(state, { type: "split", sourceSessionId, targetSessionId: "tests", edge: "bottom" });
    expect(state.groups).toHaveLength(2);
    expect(members(state)[1]).toHaveLength(4);
    expect(reduceSplitGroups(state, { type: "split", sourceSessionId: "build", targetSessionId: "tests", edge: "right" })).toBe(state);
  });
  it("moves a session between groups and dissolves a source reduced to one pane", () => {
    const moved = reduceSplitGroups(twoGroups, { type: "split", sourceSessionId: "build", targetSessionId: "tests", edge: "bottom" });
    expect(members(moved)).toEqual([["tests", "build", "monitor"]]);
    expect(moved.groups[0]!.number).toBe(2);
    expect(activeSplitView(reduceSplitGroups(moved, { type: "select", sessionId: "logs" })).root).toEqual({ sessionId: "logs" });
  });
  it("keeps two surviving panes and their focus when moving out of a larger group", () => {
    const larger = reduceSplitGroups(pair, { type: "split", sourceSessionId: "checks", targetSessionId: "logs", edge: "bottom" });
    const state = createGroup(larger, "tests", "monitor");
    const moved = reduceSplitGroups(state, { type: "split", sourceSessionId: "checks", targetSessionId: "tests", edge: "right" });
    expect(members(moved)).toEqual([["build", "logs"], ["tests", "checks", "monitor"]]);
    expect(moved.groups[0]!.view.activeSessionId).toBe("build");
  });
  it("dissolves active and inactive groups without losing the displayed session", () => {
    const inactive = reduceSplitGroups(twoGroups, { type: "dissolve-group", number: 1 });
    expect(inactive.activeGroupNumber).toBe(2);
    expect(activeSplitView(inactive)).toBe(activeSplitView(twoGroups));
    const active = reduceSplitGroups(inactive, { type: "dissolve-group", number: 2 });
    expect(active.groups).toEqual([]);
    expect(active.activeGroupNumber).toBeNull();
    expect(activeSplitView(active).root).toEqual({ sessionId: "monitor" });
    expect(reduceSplitGroups(active, { type: "dissolve-group", number: 2 })).toBe(active);
  });
  it("dissolves the group when closing its second-to-last pane", () => {
    const closed = reduceSplitGroups(twoGroups, { type: "close", sessionId: "monitor" });
    expect(members(closed)).toEqual([["build", "logs"]]);
    expect(activeSplitView(closed).root).toEqual({ sessionId: "tests" });
    expect(closed.activeGroupNumber).toBeNull();
  });
  it("removes ended sessions from all groups, including groups that are not displayed", () => {
    const single = reduceSplitGroups(twoGroups, { type: "select", sessionId: "spare" });
    const pruned = reduceSplitGroups(single, { type: "prune", sessionIds: new Set(["logs", "tests", "monitor"]) });
    expect(pruned.groups).toEqual([]);
    expect(activeSplitView(pruned).root).toEqual({ sessionId: "spare" });
    const allClosed = reduceSplitGroups(twoGroups, { type: "prune", sessionIds: new Set(["build", "logs", "tests", "monitor"]) });
    expect(allClosed.groups).toEqual([]);
    expect(activeSplitView(allClosed).root).toBeNull();
  });
  it("keeps group numbers stable and never reuses a dissolved number", () => {
    const dissolved = reduceSplitGroups(twoGroups, { type: "dissolve-group", number: 1 });
    const created = createGroup(dissolved, "build", "logs");
    expect(created.groups.map((group) => group.number)).toEqual([2, 3]);
    const closed = reduceSplitGroups(created, { type: "close", sessionId: "logs" });
    expect(createGroup(closed, "build", "logs").groups.map((group) => group.number)).toEqual([2, 4]);
  });
  it("limits group creation while allowing additions, transfers and reuse of freed capacity", () => {
    let full = emptySplitGroups;
    for (let index = 0; index < maximumSplitGroups; index++) full = createGroup(full, `a${index}`, `b${index}`);
    const single = reduceSplitGroups(full, { type: "select", sessionId: "spare" });
    expect(reduceSplitGroups(single, { type: "split", sourceSessionId: "extra", targetSessionId: "spare", edge: "right" })).toBe(single);
    const firstSelected = reduceSplitGroups(full, { type: "select-group", number: 1 });
    const largerSource = reduceSplitGroups(firstSelected, { type: "split", sourceSessionId: "extra", targetSessionId: "a0", edge: "right" });
    const fromLargerSource = reduceSplitGroups(largerSource, { type: "select", sessionId: "spare" });
    expect(reduceSplitGroups(fromLargerSource, { type: "split", sourceSessionId: "a0", targetSessionId: "spare", edge: "right" })).toBe(fromLargerSource);
    const moved = reduceSplitGroups(single, { type: "split", sourceSessionId: "a0", targetSessionId: "spare", edge: "right" });
    expect(moved.groups).toHaveLength(maximumSplitGroups);
    expect(moved.activeGroupNumber).toBe(maximumSplitGroups + 1);
    expect(reduceSplitGroups(full, { type: "split", sourceSessionId: "extra", targetSessionId: `a${maximumSplitGroups - 1}`, edge: "right" }).groups).toHaveLength(maximumSplitGroups);
    const freed = reduceSplitGroups(full, { type: "dissolve-group", number: 1 });
    expect(createGroup(freed, "spare", "extra").groups).toHaveLength(maximumSplitGroups);
  });
});
