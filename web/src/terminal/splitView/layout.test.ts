import { describe, expect, it } from "vitest";
import { emptySplitView, paneSessionIds, reduceSplitView, type PaneEdge } from "./layout";
import { nearestPaneEdge, splitViewGeometry } from "./geometry";

const single = reduceSplitView(emptySplitView, { type: "select", sessionId: "one" });
const pair = reduceSplitView(single, { type: "split", sourceSessionId: "two", targetSessionId: "one", edge: "right" });
const three = reduceSplitView(pair, { type: "split", sourceSessionId: "three", targetSessionId: "two", edge: "bottom" });
const four = reduceSplitView(three, { type: "split", sourceSessionId: "four", targetSessionId: "one", edge: "top" });

describe("split view layout", () => {
  it.each<{ edge: PaneEdge; direction: string; order: string[] }>([
    { edge: "left", direction: "horizontal", order: ["two", "one"] },
    { edge: "right", direction: "horizontal", order: ["one", "two"] },
    { edge: "top", direction: "vertical", order: ["two", "one"] },
    { edge: "bottom", direction: "vertical", order: ["one", "two"] },
  ])("splits at the $edge edge and selects the added session", ({ edge, direction, order }) => {
    const state = reduceSplitView(single, { type: "split", sourceSessionId: "two", targetSessionId: "one", edge });
    expect(paneSessionIds(state.root)).toEqual(order);
    expect(state.root).toMatchObject({ direction, ratio: 50 });
    expect(state.activeSessionId).toBe("two");
  });
  it("rejects a fifth pane but allows moving an existing pane at the limit", () => {
    expect(reduceSplitView(four, { type: "split", sourceSessionId: "five", targetSessionId: "one", edge: "right" })).toBe(four);
    const moved = reduceSplitView(four, { type: "split", sourceSessionId: "four", targetSessionId: "three", edge: "bottom" });
    expect(paneSessionIds(moved.root)).toEqual(["one", "two", "three", "four"]);
  });
  it("ignores self drops and missing targets", () => {
    for (const targetSessionId of ["one", "missing"]) {
      expect(reduceSplitView(single, { type: "split", sourceSessionId: "one", targetSessionId, edge: "left" })).toBe(single);
    }
  });
  it("closes a nested pane and expands its sibling without changing the other split", () => {
    const closed = reduceSplitView(three, { type: "close", sessionId: "two" });
    expect(closed.root).toEqual({ direction: "horizontal", ratio: 50, first: { sessionId: "one" }, second: { sessionId: "three" } });
    expect(closed.activeSessionId).toBe("three");
  });
  it("selects the survivor when the selected pane is closed and keeps the last pane", () => {
    const closed = reduceSplitView({ ...pair, focusMode: true }, { type: "close", sessionId: "two" });
    expect(closed.root).toEqual(single.root);
    expect(closed.activeSessionId).toBe("one");
    expect(closed.focusMode).toBe(false);
    expect(reduceSplitView(closed, { type: "close", sessionId: "one" })).toBe(closed);
  });
  it("swaps two panes while preserving their sizes and selected session", () => {
    const resized = reduceSplitView(three, { type: "resize", path: ["second"], ratio: 70 });
    const swapped = reduceSplitView(resized, { type: "swap", sourceSessionId: "one", targetSessionId: "three" });
    expect(paneSessionIds(swapped.root)).toEqual(["three", "two", "one"]);
    expect(swapped.root).toMatchObject({ ratio: 50, second: { ratio: 70 } });
    expect(swapped.activeSessionId).toBe("three");
    expect(reduceSplitView(swapped, { type: "swap", sourceSessionId: "missing", targetSessionId: "three" })).toBe(swapped);
  });
  it.each([[1, 10], [99, 90], [54.6, 55], [NaN, 50], [Infinity, 50]])("clamps a requested ratio of %s to %s", (ratio, expected) => {
    expect(reduceSplitView(pair, { type: "resize", path: [], ratio }).root).toMatchObject({ ratio: expected });
  });
  it("removes all ended or deleted sessions in one transition", () => {
    const pruned = reduceSplitView({ ...four, focusMode: true }, { type: "prune", sessionIds: new Set(["four", "two", "three"]) });
    expect(pruned.activeSessionId).toBe("one");
    expect(pruned.root).toEqual(single.root);
    expect(pruned.focusMode).toBe(false);
    expect(reduceSplitView(pair, { type: "prune", sessionIds: new Set(["one", "two"]) }).root).toBeNull();
  });
  it("keeps a standalone ended session available for inspecting its output", () => {
    expect(reduceSplitView(single, { type: "prune", sessionIds: new Set(["one"]) })).toBe(single);
  });
  it("selects a member without changing layout and ends the split for a nonmember", () => {
    expect(reduceSplitView(pair, { type: "select", sessionId: "one" }).root).toBe(pair.root);
    const selected = reduceSplitView({ ...pair, focusMode: true }, { type: "select", sessionId: "other" });
    expect(selected.root).toEqual({ sessionId: "other" });
    expect(selected.focusMode).toBe(false);
  });
  it("places nested panes and dividers relative to their own split", () => {
    const geometry = splitViewGeometry(three.root);
    expect(geometry.panes).toEqual([
      { sessionId: "one", style: { left: "0%", top: "0%", width: "50%", height: "100%" } },
      { sessionId: "two", style: { left: "50%", top: "0%", width: "50%", height: "50%" } },
      { sessionId: "three", style: { left: "50%", top: "50%", width: "50%", height: "50%" } },
    ]);
    expect(geometry.dividers.map((divider) => divider.path)).toEqual([[], ["second"]]);
    expect(nearestPaneEdge({ x: 590, y: 100 }, { width: 600, height: 400 })).toBe("right");
  });
});
