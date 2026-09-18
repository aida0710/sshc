import { describe, expect, it } from "vitest";
import { addTab, blankTab, closeTab, maxTabsPerPane, moveTab, paneOf, selectTab, type SFTPPane, type SFTPTab } from "./sftpPanes";

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
