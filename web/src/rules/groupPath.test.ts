import { describe, expect, it } from "vitest";
import { groupAncestry, groupBaseName, groupDepth, isGroupOrDescendant, parentGroupName, renamedGroup } from "./groupPath";

describe("group paths", () => {
  it("reads a group's folders the way the engine does", () => {
    expect(groupDepth("")).toBe(0);
    expect(groupDepth("work/eu")).toBe(2);
    expect(parentGroupName("work/eu")).toBe("work");
    expect(parentGroupName("work")).toBe("");
    expect(groupBaseName("work/eu")).toBe("eu");
    expect(groupAncestry("work/eu/db")).toEqual(["work/eu/db", "work/eu", "work"]);
  });

  it("counts only whole folders as inside a group", () => {
    expect(isGroupOrDescendant("work", "work")).toBe(true);
    expect(isGroupOrDescendant("work/eu", "work")).toBe(true);
    expect(isGroupOrDescendant("workshop", "work")).toBe(false);
  });

  it("moves the groups inside a renamed group with it", () => {
    expect(renamedGroup("work/eu", "work", "office")).toBe("office/eu");
    expect(renamedGroup("work", "work", "office")).toBe("office");
    expect(renamedGroup("workshop", "work", "office")).toBeNull();
  });
});
