import { describe, expect, it } from "vitest";
import type { Overview } from "../api/config";
import { groupAfterRename, movedHostIdentity } from "./connectionMoves";

const overview = {
  hosts: [
    { identity: { path: "config", alias: "web" }, group: "prod/web" },
    { identity: { path: "config.d/db", alias: "db" }, group: "prod" },
  ],
} as unknown as Overview;

describe("movedHostIdentity", () => {
  it("finds the host under the group it moved into", () => {
    expect(movedHostIdentity(overview, "db", "prod")).toEqual({ path: "config.d/db", alias: "db" });
  });

  it("is undefined when the refreshed overview does not show the host there", () => {
    expect(movedHostIdentity(overview, "db", "staging")).toBeUndefined();
    expect(movedHostIdentity(null, "db", "prod")).toBeUndefined();
  });
});

describe("groupAfterRename", () => {
  it("follows the selected group and everything below it", () => {
    expect(groupAfterRename("prod", "prod", "live")).toBe("live");
    expect(groupAfterRename("prod/web", "prod", "live")).toBe("live/web");
  });

  it("leaves a selection outside the renamed subtree alone", () => {
    expect(groupAfterRename("production", "prod", "live")).toBeNull();
    expect(groupAfterRename(undefined, "prod", "live")).toBeNull();
  });
});
