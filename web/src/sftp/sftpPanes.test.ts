import { describe, expect, it } from "vitest";
import { addTab, blankTab, closeTab, maxTabsPerPane, moveTab, paneOf, selectTab, swapPanes, type SFTPPane, type SFTPTab } from "./sftpPanes";

function tab(alias: string, path = "/"): SFTPTab {
  return { ...blankTab(), alias, path };
}

function names(pane: SFTPPane | undefined): string[] {
  return (pane?.tabs ?? []).map((item) => item.alias);
}

describe("SFTP panes", () => {
  it("splits the only pane by moving a tab beside it and leaves a blank tab behind", () => {
    const edge = tab("edge");
    const panes = [paneOf([edge])];

    const split = moveTab(panes, edge.id, { side: "right" });

    expect(split).toHaveLength(2);
    expect(names(split[0])).toEqual([""]);
    expect(names(split[1])).toEqual(["edge"]);
    expect(split[1]?.activeId).toBe(edge.id);
    expect(split[0]?.id).toBe(panes[0]?.id);
  });

  it("puts the moved tab on the side it was dropped on", () => {
    const edge = tab("edge");
    const nas = tab("nas");
    const panes = [paneOf([edge, nas], nas.id)];

    const split = moveTab(panes, nas.id, { side: "left" });

    expect(names(split[0])).toEqual(["nas"]);
    expect(names(split[1])).toEqual(["edge"]);
    expect(split[1]?.activeId).toBe(edge.id);
  });

  it("refuses a third pane", () => {
    const edge = tab("edge");
    const panes = [paneOf([edge, tab("nas")]), paneOf([tab("miyabi")])];

    expect(moveTab(panes, edge.id, { side: "left" })).toBe(panes);
  });

  it("moves a tab into the other pane and drops a pane whose last tab left", () => {
    const edge = tab("edge");
    const nas = tab("nas");
    const panes = [paneOf([edge]), paneOf([nas])];

    const merged = moveTab(panes, nas.id, { paneId: panes[0]!.id });

    expect(merged).toHaveLength(1);
    expect(names(merged[0])).toEqual(["edge", "nas"]);
    expect(merged[0]?.activeId).toBe(nas.id);
  });

  it("moves a tab into the other pane at the place it was dropped", () => {
    const edge = tab("edge");
    const nas = tab("nas");
    const miyabi = tab("miyabi");
    const panes = [paneOf([edge, nas]), paneOf([miyabi])];

    const moved = moveTab(panes, miyabi.id, { paneId: panes[0]!.id, index: 1 });

    expect(moved).toHaveLength(1);
    expect(names(moved[0])).toEqual(["edge", "miyabi", "nas"]);
    expect(moved[0]?.activeId).toBe(miyabi.id);
  });

  it("reorders a tab within its pane, counting the place among the tabs before the move", () => {
    const [a, b, c, d] = [tab("a"), tab("b"), tab("c"), tab("d")];
    const panes = [paneOf([a, b, c, d], b.id)];
    const paneId = panes[0]!.id;

    expect(names(moveTab(panes, a.id, { paneId, index: 3 })[0])).toEqual(["b", "c", "a", "d"]);
    expect(names(moveTab(panes, d.id, { paneId, index: 0 })[0])).toEqual(["d", "a", "b", "c"]);
    expect(names(moveTab(panes, b.id, { paneId, index: 4 })[0])).toEqual(["a", "c", "d", "b"]);
    expect(moveTab(panes, b.id, { paneId, index: 4 })[0]?.activeId).toBe(b.id);
    expect(moveTab(panes, b.id, { paneId, index: 2 })).toBe(panes);
    expect(moveTab(panes, b.id, { paneId })).toBe(panes);
  });

  it("swaps two panes, each keeping its tabs and selection", () => {
    const edge = tab("edge");
    const nas = tab("nas");
    const miyabi = tab("miyabi");
    const panes = [paneOf([edge, nas], nas.id), paneOf([miyabi])];

    const swapped = swapPanes(panes);

    expect(names(swapped[0])).toEqual(["miyabi"]);
    expect(names(swapped[1])).toEqual(["edge", "nas"]);
    expect(swapped[1]?.activeId).toBe(nas.id);
    expect(swapPanes([paneOf([edge])])).toHaveLength(1);
  });

  it("keeps a tab where it is when the other pane is full", () => {
    const edge = tab("edge");
    const full = paneOf(Array.from({ length: maxTabsPerPane }, (_, index) => tab(`host${index}`)));
    const panes = [paneOf([edge]), full];

    expect(moveTab(panes, edge.id, { paneId: full.id })).toBe(panes);
    expect(addTab(panes, full.id, blankTab())).toBe(panes);
  });

  it("selects the neighbour of a closed tab and never leaves the last pane empty", () => {
    const first = tab("first");
    const second = tab("second");
    const third = tab("third");
    const panes = [paneOf([first, second, third], second.id)];

    const closed = closeTab(panes, second.id);
    expect(names(closed[0])).toEqual(["first", "third"]);
    expect(closed[0]?.activeId).toBe(third.id);

    const emptied = closeTab(closeTab(closed, third.id), first.id);
    expect(emptied).toHaveLength(1);
    expect(names(emptied[0])).toEqual([""]);
    expect(emptied[0]?.id).toBe(panes[0]?.id);
  });

  it("returns the same layout when nothing changes", () => {
    const edge = tab("edge");
    const panes = [paneOf([edge])];

    expect(selectTab(panes, edge.id)).toBe(panes);
    expect(closeTab(panes, "missing")).toBe(panes);
    expect(moveTab(panes, edge.id, { paneId: panes[0]!.id })).toBe(panes);
  });
});
