import { describe, expect, it } from "vitest";
import type { HostEntry, Overview } from "../api/config";
import {
  buildConnectionBrowserIndex,
  projectConnectionBrowser,
} from "./connectionBrowser";

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
        favourite: true,
        tags: ["storage", "lan"],
        colour: "#f97316",
      },
      { identity: workNas.identity, order: -1 },
      { identity: euApi.identity, favourite: true, tags: ["production"] },
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
      favourite: true,
      colour: "#f97316",
      duplicateAlias: true,
    });
    expect(index.servers[1]!.duplicateAlias).toBe(true);
    expect(index.duplicateAliases).toEqual(new Set(["nas"]));
  });

  it("projects one declared group level at a time and preserves empty groups", () => {
    const index = buildConnectionBrowserIndex(overview);
    const root = projectConnectionBrowser(index, { view: "groups", scope: "root" }, "", false);
    const home = projectConnectionBrowser(
      index,
      { view: "groups", scope: "named", group: "home" },
      "",
      false,
    );
    const eu = projectConnectionBrowser(
      index,
      { view: "groups", scope: "named", group: "home/eu" },
      "",
      false,
    );

    expect(root).toMatchObject({
      kind: "group-level",
      group: null,
      groups: [
        { name: "home", descendantCount: 2 },
        { name: "work", descendantCount: 1 },
        { name: "empty", descendantCount: 0 },
        { name: "hidden/east", descendantCount: 1 },
        { name: "hidden-direct", descendantCount: 1 },
      ],
      servers: [],
      ungroupedCount: 1,
    });
    expect(home).toMatchObject({
      kind: "group-level",
      group: "home",
      groups: [{ name: "home/eu", descendantCount: 1 }],
      servers: [{ identity: { alias: "nas" } }],
    });
    expect(eu).toMatchObject({
      kind: "group-level",
      group: "home/eu",
      groups: [],
      servers: [{ identity: { alias: "eu-api" } }],
    });
  });

  it("uses Overview.groups as vocabulary and never invents metadata or host groups", () => {
    const index = buildConnectionBrowserIndex(overview);

    expect(index.groups.map((group) => group.name)).not.toContain("metadata-only");
    expect(index.groups.map((group) => group.name)).not.toContain("not-declared");
    expect(projectConnectionBrowser(
      index,
      { view: "groups", scope: "named", group: "metadata-only" },
      "",
      false,
    )).toEqual({ kind: "missing-group", group: "metadata-only" });
  });

  it("recursively searches a named scope and includes full group paths", () => {
    const index = buildConnectionBrowserIndex(overview);

    expect(projectConnectionBrowser(
      index,
      { view: "groups", scope: "named", group: "home" },
      "eu-api",
      false,
    )).toMatchObject({
      kind: "search-results",
      scope: "home",
      servers: [{ identity: { alias: "eu-api" }, group: "home/eu" }],
    });
    expect(projectConnectionBrowser(
      index,
      { view: "groups", scope: "root" },
      "home/eu",
      false,
    )).toMatchObject({
      kind: "search-results",
      scope: null,
      servers: [{ identity: { alias: "eu-api" }, group: "home/eu" }],
    });
  });

  it("filters direct servers and group cards by favourite descendants", () => {
    const index = buildConnectionBrowserIndex(overview);
    const root = projectConnectionBrowser(index, { view: "groups", scope: "root" }, "", true);
    const home = projectConnectionBrowser(
      index,
      { view: "groups", scope: "named", group: "home" },
      "",
      true,
    );

    expect(root).toMatchObject({
      kind: "group-level",
      groups: [{ name: "home", favouriteDescendantCount: 2 }],
      servers: [],
      ungroupedCount: 0,
    });
    expect(home).toMatchObject({
      kind: "group-level",
      groups: [{ name: "home/eu", favouriteDescendantCount: 1 }],
      servers: [{ identity: { alias: "nas" } }],
    });
  });

  it("keeps a filtered zero-result projection distinct from missing data", () => {
    const index = buildConnectionBrowserIndex(overview);

    expect(projectConnectionBrowser(index, { view: "servers" }, "no-such-server", false)).toEqual({
      kind: "servers",
      servers: [],
    });
    expect(index.servers).not.toHaveLength(0);
  });
});
