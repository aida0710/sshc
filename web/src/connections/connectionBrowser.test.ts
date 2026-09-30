import { describe, expect, it } from "vitest";
import type { HostEntry, HostIdentity, Overview } from "../api/config";
import { buildConnectionBrowserIndex, identityKey } from "./connectionBrowser";

function host(path: string, alias: string, group?: string): HostEntry {
  return {
    identity: { path, alias },
    file: { path, absolute: `/home/tester/.ssh/${path}` },
    line: 1,
    patterns: alias === "" ? ["*"] : [alias],
    ...(alias === "" ? { wildcard: true } : {}),
    editable: true,
    ...(group === undefined ? {} : { group }),
  };
}

const homeNas = host("connections/home/nas.conf", "nas", "home");
const workNas = host("connections/work/nas.conf", "nas", "work");
const bastion = host("config", "bastion");
const euApi = host("connections/home/eu/api.conf", "eu-api", "home/eu");
const east = host("connections/hidden/east/app.conf", "east-app", "hidden/east");
const hiddenDirect = host("connections/hidden-direct/db.conf", "hidden-db", "hidden-direct");
const undeclared = host("connections/orphan.conf", "orphan", "not-declared");
const patternRule = host("config", "");

const overview: Overview = {
  entry: { path: "config", absolute: "/home/tester/.ssh/config" },
  files: [],
  hosts: [bastion, euApi, homeNas, workNas, east, hiddenDirect, undeclared, patternRule],
  groups: [
    {
      name: "home",
      directory: "connections/home",
      keyDirectory: "keys/home",
      memberCount: 1,
      directoryPresent: true,
    },
    {
      name: "home/eu",
      parent: "home",
      directory: "connections/home/eu",
      keyDirectory: "keys/home/eu",
      memberCount: 1,
      directoryPresent: true,
    },
    {
      name: "work",
      directory: "connections/work",
      keyDirectory: "keys/work",
      memberCount: 1,
      directoryPresent: true,
    },
    {
      name: "empty",
      directory: "connections/empty",
      keyDirectory: "keys/empty",
      memberCount: 0,
      directoryPresent: true,
    },
    {
      name: "hidden",
      directory: "connections/hidden",
      keyDirectory: "keys/hidden",
      memberCount: 0,
      directoryPresent: true,
    },
    {
      name: "hidden/east",
      parent: "hidden",
      directory: "connections/hidden/east",
      keyDirectory: "keys/hidden/east",
      memberCount: 1,
      directoryPresent: true,
    },
    {
      name: "hidden-direct",
      directory: "connections/hidden-direct",
      keyDirectory: "keys/hidden-direct",
      memberCount: 1,
      directoryPresent: true,
    },
  ],
  metadata: {
    schemaVersion: 2,
    groups: [
      { name: "home", order: 1 },
      { name: "home/eu", order: 2 },
      { name: "work", order: 3 },
      { name: "empty", order: 4 },
      { name: "hidden", order: 5, hidden: true },
      { name: "hidden/east", order: 6 },
      { name: "hidden-direct", order: 7, hidden: true },
      { name: "metadata-only", order: -100 },
    ],
    hosts: [
      {
        identity: homeNas.identity,
        order: -2,
        tags: ["storage", "lan"],
        colour: "#f97316",
      },
      { identity: workNas.identity, order: -1 },
      { identity: euApi.identity, tags: ["production"] },
    ],
  },
  diagnostics: [],
  notices: [],
};

describe("connection browser index", () => {
  it("indexes only concrete servers in stable metadata order and marks duplicate aliases", () => {
    const index = buildConnectionBrowserIndex(overview);

    expect(index.servers.map((server) => [server.identity.path, server.identity.alias])).toEqual([
      ["connections/home/nas.conf", "nas"],
      ["connections/work/nas.conf", "nas"],
      ["config", "bastion"],
      ["connections/home/eu/api.conf", "eu-api"],
      ["connections/hidden/east/app.conf", "east-app"],
      ["connections/hidden-direct/db.conf", "hidden-db"],
      ["connections/orphan.conf", "orphan"],
    ]);
    expect(index.servers[0]).toMatchObject({
      group: "home",
      tags: ["storage", "lan"],
      colour: "#f97316",
      duplicateAlias: true,
    });
    expect(index.servers[1]!.duplicateAlias).toBe(true);
    expect(index.duplicateAliases).toEqual(new Set(["nas"]));
  });

  it("indexes visible group levels and preserves empty groups", () => {
    const index = buildConnectionBrowserIndex(overview);

    expect(index.visibleChildrenByParent.get("")).toMatchObject([
      { name: "home", descendantCount: 2 },
      { name: "work", descendantCount: 1 },
      { name: "empty", descendantCount: 0 },
      { name: "hidden/east", descendantCount: 1 },
      { name: "hidden-direct", descendantCount: 1 },
    ]);
    expect(index.visibleChildrenByParent.get("home")).toMatchObject([
      { name: "home/eu", descendantCount: 1 },
    ]);
    expect(index.visibleChildrenByParent.get("home/eu")).toEqual([]);
  });

  it("uses Overview.groups as vocabulary and never invents metadata or host groups", () => {
    const index = buildConnectionBrowserIndex(overview);

    expect(index.groups.map((group) => group.name)).not.toContain("metadata-only");
    expect(index.groups.map((group) => group.name)).not.toContain("not-declared");
    expect(index.groupByName.has("metadata-only")).toBe(false);
  });
});

describe("ホストの識別子を鍵にする", () => {
  it("path か alias が違えば、別の鍵になる", () => {
    const base = identityKey({ path: "config", alias: "web" });
    expect(identityKey({ path: "config", alias: "web" })).toBe(base);
    expect(identityKey({ path: "work/config", alias: "web" })).not.toBe(base);
    expect(identityKey({ path: "config", alias: "web2" })).not.toBe(base);
  });

  it("path と alias の境目を跨いで衝突しない", () => {
    expect(identityKey({ path: "a/b", alias: "c" })).not.toBe(identityKey({ path: "a", alias: "b/c" }));
  });

  it("鍵は identity の全項目を綴っている", () => {
    const keyed: Record<keyof HostIdentity, true> = { path: true, alias: true };
    expect(Object.keys(keyed).sort()).toEqual(["alias", "path"]);
  });
});
