import { describe, expect, it } from "vitest";
import { applySelectionGesture, emptyMultiSelection, keepVisible } from "./multiSelection";

const order = ["a", "b", "c", "d", "e"];

describe("applySelectionGesture", () => {
  it("counts the open connection as chosen when Ctrl adds the first other one", () => {
    const next = applySelectionGesture({ current: emptyMultiSelection, clicked: "c", opened: "a", visibleOrder: order, gesture: "toggle" });
    expect([...next.keys]).toEqual(["a", "c"]);
    expect(next.anchor).toBe("c");
  });

  it("takes a chosen connection back out on a second Ctrl click", () => {
    const chosen = { keys: new Set(["a", "c"]), anchor: "c" };
    const next = applySelectionGesture({ current: chosen, clicked: "c", opened: "a", visibleOrder: order, gesture: "toggle" });
    expect([...next.keys]).toEqual(["a"]);
  });

  it("chooses everything between the open connection and a Shift click in screen order", () => {
    const next = applySelectionGesture({ current: emptyMultiSelection, clicked: "d", opened: "b", visibleOrder: order, gesture: "range" });
    expect([...next.keys]).toEqual(["b", "c", "d"]);
  });

  it("counts a Shift range upwards from the last Ctrl click", () => {
    const chosen = { keys: new Set(["d"]), anchor: "d" };
    const next = applySelectionGesture({ current: chosen, clicked: "a", opened: null, visibleOrder: order, gesture: "range" });
    expect([...next.keys]).toEqual(["a", "b", "c", "d"]);
    expect(next.anchor).toBe("d");
  });

  it("chooses only the clicked connection when the anchor is no longer on screen", () => {
    const chosen = { keys: new Set(["z"]), anchor: "z" };
    const next = applySelectionGesture({ current: chosen, clicked: "b", opened: null, visibleOrder: order, gesture: "range" });
    expect([...next.keys]).toEqual(["b"]);
  });
});

describe("keepVisible", () => {
  it("drops connections a filter hid, and the anchor with them", () => {
    const chosen = { keys: new Set(["a", "c"]), anchor: "c" };
    const next = keepVisible(chosen, new Set(["a", "b"]));
    expect([...next.keys]).toEqual(["a"]);
    expect(next.anchor).toBeNull();
  });

  it("returns the same selection when everything chosen is still visible", () => {
    const chosen = { keys: new Set(["a"]), anchor: "a" };
    expect(keepVisible(chosen, new Set(order))).toBe(chosen);
  });
});
