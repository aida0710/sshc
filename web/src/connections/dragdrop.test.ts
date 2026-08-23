import { describe, expect, it } from "vitest";
import { canDrop, type DragPayload } from "./dragdrop";

const groups = ["home", "work", "work/eu", "client-a"];
const nas: DragPayload = {
  kind: "connection",
  path: "connections/home/nas.conf",
  alias: "nas",
  group: "home",
};
const work: DragPayload = { kind: "group", name: "work" };

describe("canDrop, for a connection", () => {
  it("accepts another group", () => {
    expect(canDrop(nas, "work", groups)).toBe(true);
  });

  it("accepts the no-group heading", () => {
    expect(canDrop(nas, "", groups)).toBe(true);
  });

  it("refuses the group it is already in, because there is nothing to do", () => {
    expect(canDrop(nas, "home", groups)).toBe(false);
  });

  it("refuses the no-group heading when it is ungrouped already", () => {
    const loose: DragPayload = { kind: "connection", path: "config", alias: "bastion", group: "" };
    expect(canDrop(loose, "", groups)).toBe(false);
  });
});

describe("canDrop, for a group", () => {
  it("accepts another group as a new parent", () => {
    expect(canDrop(work, "client-a", groups)).toBe(true);
  });

  it("refuses itself", () => {
    expect(canDrop(work, "work", groups)).toBe(false);
  });

  it("refuses its own descendant", () => {
    expect(canDrop(work, "work/eu", groups)).toBe(false);
  });

  it("refuses a parent that already holds a group of that name", () => {
    expect(canDrop({ kind: "group", name: "client-a/work" }, "", ["client-a/work", "work"])).toBe(false);
  });

  it("accepts the no-group heading for a nested group", () => {
    expect(canDrop({ kind: "group", name: "work/eu" }, "", groups)).toBe(true);
  });

  it("refuses the no-group heading for a group already at the top", () => {
    expect(canDrop(work, "", groups)).toBe(false);
  });

  it("refuses a nesting deeper than six segments", () => {
    const deep = ["a", "a/b/c/d/e/f"];
    expect(canDrop({ kind: "group", name: "a" }, "a/b/c/d/e/f", deep)).toBe(false);
  });

  it("accepts a nesting that lands exactly on the sixth segment", () => {
    const deep = ["a", "b/c/d/e/f"];
    expect(canDrop({ kind: "group", name: "a" }, "b/c/d/e/f", deep)).toBe(true);
  });
});
