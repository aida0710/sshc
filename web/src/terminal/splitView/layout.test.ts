import { describe, expect, it } from "vitest";
import { emptySplitView, paneSessionIds, reduceSplitView, type PaneEdge, type SplitViewNode } from "./layout";
import { fitSplitRatio } from "../../ui/SplitResizeHandle";
import { minimumExtentPx, minimumPanePx, nearestPaneEdge, paneDropZone, splitViewGeometry, type SplitArea } from "./geometry";

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
    const geometry = splitViewGeometry(three.root, null);
    expect(geometry.panes).toEqual([
      { sessionId: "one", style: { left: "0%", top: "0%", width: "50%", height: "100%" } },
      { sessionId: "two", style: { left: "50%", top: "0%", width: "50%", height: "50%" } },
      { sessionId: "three", style: { left: "50%", top: "50%", width: "50%", height: "50%" } },
    ]);
    expect(geometry.dividers.map((divider) => divider.path)).toEqual([[], ["second"]]);
    expect(nearestPaneEdge({ x: 590, y: 100 }, { width: 600, height: 400 })).toBe("right");
  });
});

describe("how small a split pane can be dragged", () => {
  it("adds up panes along a split and takes the largest across it", () => {
    // four: one and four are stacked on the left, two and three on the right.
    expect(minimumExtentPx(four.root!, "vertical")).toBe(2 * minimumPanePx.height);
    expect(minimumExtentPx(four.root!, "horizontal")).toBe(2 * minimumPanePx.width);
    const { dividers } = splitViewGeometry(four.root, null);
    expect(dividers[0]?.minimum).toEqual({ first: minimumPanePx.width, second: minimumPanePx.width });
  });

  it("keeps both halves at their minimum in px instead of 10% of a nested split", () => {
    // Two panes stacked in the top 10% of a 760px split: the inner split is 76px tall.
    const minimum = { first: minimumPanePx.height, second: minimumPanePx.height };
    expect(fitSplitRatio(10, 760, minimum)).toBe(11);
    expect(fitSplitRatio(95, 760, minimum)).toBe(89);
    expect(fitSplitRatio(40, 760, minimum)).toBe(40);
    // When neither minimum fits, the two share the room.
    expect(fitSplitRatio(10, 100, minimum)).toBe(50);
    expect(fitSplitRatio(10, 100, { first: 3 * minimumPanePx.height, second: minimumPanePx.height })).toBe(75);
  });
});

// panePx は、area の広さに描いた各ペインの幅と高さ（px）である。
function panePx(root: SplitViewNode | null, area: SplitArea) {
  return splitViewGeometry(root, area).panes.map(({ sessionId, style }) => ({
    sessionId,
    width: parseFloat(String(style.width)) / 100 * area.width,
    height: parseFloat(String(style.height)) / 100 * area.height,
  }));
}

// splitInRow は、one の右に two、two の右に three と、count 個のペインを1つずつ右へ足した分割である。
function splitInRow(count: number) {
  const ids = ["one", "two", "three", "four"].slice(0, count);
  return ids.slice(1).reduce((state, sessionId, index) => reduceSplitView(state, { type: "split", sourceSessionId: sessionId, targetSessionId: ids[index]!, edge: "right" }), single);
}

describe("how a pane added to a split is sized", () => {
  it("divides the row it joins evenly, instead of halving the pane it goes beside", () => {
    const widths = panePx(splitInRow(4).root, { width: 1000, height: 600 }).map((pane) => Math.round(pane.width));
    expect(widths).toEqual([250, 250, 250, 250]);
  });

  it("leaves a split across the row as it was", () => {
    // one | (two / three / four)。右の列は3つに均等に分け、横の並びは渡した 70% のままにする。
    const column = reduceSplitView(reduceSplitView(pair, { type: "resize", path: [], ratio: 70 }), { type: "split", sourceSessionId: "three", targetSessionId: "two", edge: "bottom" });
    const added = reduceSplitView(column, { type: "split", sourceSessionId: "four", targetSessionId: "three", edge: "bottom" });
    expect(added.root).toMatchObject({ ratio: 70 });
    const area = { width: 1000, height: 900 };
    const heights = panePx(added.root, area).filter((pane) => pane.sessionId !== "one").map((pane) => pane.height);
    // 描くときは比率を整数の％に丸めるので、1%（9px）までずれる。
    for (const height of heights) expect(Math.abs(height - area.height / 3)).toBeLessThanOrEqual(area.height / 100);
  });

  it("refuses a pane that would leave some pane less than its minimum, on that side only", () => {
    const row = splitInRow(3);
    const area = { width: 3 * minimumPanePx.width + 10, height: 600 };
    expect(reduceSplitView(row, { type: "split", sourceSessionId: "four", targetSessionId: "three", edge: "right", area })).toBe(row);
    const below = reduceSplitView(row, { type: "split", sourceSessionId: "four", targetSessionId: "three", edge: "bottom", area });
    expect(paneSessionIds(below.root)).toEqual(["one", "two", "three", "four"]);
  });

  it("gives every pane its minimum in an area that holds exactly the minimums", () => {
    const area = { width: 4 * minimumPanePx.width, height: minimumPanePx.height };
    for (const pane of panePx(splitInRow(4).root, area)) expect(pane.width).toBeGreaterThanOrEqual(minimumPanePx.width - 1e-9);
  });
});

describe("how a split is drawn in an area narrower than its ratios allow", () => {
  it("draws each pane at least its minimum, and keeps the ratio it was given for a wider area", () => {
    const wide = reduceSplitView(pair, { type: "resize", path: [], ratio: 90 });
    const narrow = { width: 600, height: 400 };
    const [, second] = panePx(wide.root, narrow);
    expect(second!.width).toBeGreaterThanOrEqual(minimumPanePx.width);
    expect(splitViewGeometry(wide.root, narrow).dividers[0]!.ratio).toBeLessThan(90);
    expect(splitViewGeometry(wide.root, { width: 3000, height: 400 }).dividers[0]!.ratio).toBe(90);
  });
});

describe("where a pane dragged by its handle goes", () => {
  const size = { width: 600, height: 400 };
  it("swaps in the middle of another pane and moves beside an edge near one", () => {
    expect(paneDropZone({ x: 300, y: 200 }, size)).toBeNull();
    // Just inside the middle: 151px of 600 from the left, 101px of 400 from the top.
    expect(paneDropZone({ x: 151, y: 101 }, size)).toBeNull();
    expect(paneDropZone({ x: 149, y: 200 }, size)).toBe("left");
    expect(paneDropZone({ x: 300, y: 390 }, size)).toBe("bottom");
    expect(paneDropZone({ x: 590, y: 20 }, size)).toBe("right");
  });
});
