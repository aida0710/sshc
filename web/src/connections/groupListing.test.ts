import { describe, expect, it } from "vitest";
import type { Overview } from "../api/config";
import { listGroups } from "./groupListing";

const group = (name: string) => ({ name, directory: `connections/${name}`, keyDirectory: `keys/${name}`, memberCount: 0, directoryPresent: true });

describe("listGroups", () => {
  it("lists groups in the order of the group list, parents first and siblings by display order", () => {
    const overview = {
      hosts: [{ identity: { path: "connections/work/eu/a.conf", alias: "db" }, file: { path: "connections/work/eu/a.conf", absolute: "/a" }, line: 1, patterns: ["db"], editable: true, group: "work/eu" }],
      groups: [group("home"), group("work"), group("work/eu")],
      metadata: { schemaVersion: 1, groups: [{ name: "work", order: 0 }, { name: "home", order: 1 }] },
    } as unknown as Overview;
    expect(listGroups(overview)).toEqual([
      { name: "work", label: "work", depth: 0, count: 1 },
      { name: "work/eu", label: "eu", depth: 1, count: 1 },
      { name: "home", label: "home", depth: 0, count: 0 },
    ]);
  });
});
