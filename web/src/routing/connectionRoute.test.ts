import { describe, expect, it } from "vitest";
import {
  connectionLocation,
  parseConnectionLocation,
  type ConnectionBrowserLocation,
} from "./connectionRoute";

const servers: ConnectionBrowserLocation = { view: "servers" };
const namedGroup: ConnectionBrowserLocation = {
  view: "groups",
  scope: "named",
  group: "home/eu",
};

describe("connection routes", () => {
  it("redirects only the bare section root to the default server browser", () => {
    expect(parseConnectionLocation({ pathname: "/connections", search: "?tab=raw" })).toEqual({
      kind: "redirect",
      location: "/connections/servers",
    });
  });

  it("round-trips the server browser and a duplicate-safe connection identity", () => {
    const location = connectionLocation(servers, {
      path: "connections/work/api.conf",
      alias: "api prod",
      panel: "Basic",
      advanced: "Jump",
    });
    expect(location).toBe(
      "/connections/servers?path=connections%2Fwork%2Fapi.conf&host=api+prod&panel=basic",
    );
    expect(parseConnectionLocation({
      pathname: "/connections/servers",
      search: "?path=connections%2Fwork%2Fapi.conf&host=api+prod&panel=basic",
    })).toEqual({
      kind: "valid",
      browser: servers,
      target: {
        path: "connections/work/api.conf",
        alias: "api prod",
        panel: "Basic",
        advanced: "Jump",
      },
    });
  });

  it("round-trips a nested group and its advanced sub-area", () => {
    const location = connectionLocation(namedGroup, {
      path: "connections/home/eu.conf",
      alias: "münchen",
      panel: "Advanced",
      advanced: "Raw",
    });
    expect(location).toBe(
      "/connections/groups/home/eu?path=connections%2Fhome%2Feu.conf&host=m%C3%BCnchen&panel=advanced&advanced=raw",
    );
    expect(parseConnectionLocation({
      pathname: "/connections/groups/home/eu",
      search: "?path=connections%2Fhome%2Feu.conf&host=m%C3%BCnchen&panel=advanced&advanced=raw",
    })).toEqual({
      kind: "valid",
      browser: namedGroup,
      target: {
        path: "connections/home/eu.conf",
        alias: "münchen",
        panel: "Advanced",
        advanced: "Raw",
      },
    });
  });

  it("formats root, nested, and ungrouped browser locations canonically", () => {
    expect(connectionLocation(servers, null)).toBe("/connections/servers");
    expect(connectionLocation({ view: "groups", scope: "root" }, null)).toBe(
      "/connections/groups",
    );
    expect(connectionLocation(namedGroup, null)).toBe("/connections/groups/home/eu");
    expect(connectionLocation({ view: "groups", scope: "ungrouped" }, null)).toBe(
      "/connections/groups?scope=ungrouped",
    );
  });

  it.each([
    ["/connections/files", ""],
    ["/connections/groups/home/%2Fetc", ""],
    ["/connections/groups", "?scope=missing"],
    ["/connections/servers", "?scope=ungrouped"],
    ["/connections/servers", "?tab=raw"],
    ["/connections/servers", "?path=config&host=api&panel=basic&host=other"],
    ["/connections/servers", "?path=config&host=api"],
    ["/connections/servers", "?path=config&host=api&panel=advanced"],
    ["/connections/servers", "?path=config&host=api&panel=basic&advanced=raw"],
    ["/connections/servers", "?path=connections%2F..%2Fconfig&host=api&panel=basic"],
    ["/connections/servers", "?path=config&host=api%00hidden&panel=basic"],
  ] as const)("rejects a non-canonical or unsafe location %s%s", (pathname, search) => {
    expect(parseConnectionLocation({ pathname, search })).toEqual({ kind: "invalid" });
  });

});
