import { describe, expect, it } from "vitest";
import {
  connectionLocation,
  parseConnectionLocation,
} from "./connectionRoute";

describe("connection routes", () => {
  it("redirects only the bare section root to the default server browser", () => {
    expect(parseConnectionLocation({ pathname: "/connections", search: "?tab=raw" })).toEqual({
      kind: "redirect",
      location: "/connections/servers",
    });
  });

  it("round-trips the server browser and a duplicate-safe connection identity", () => {
    const location = connectionLocation({
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
      target: {
        path: "connections/work/api.conf",
        alias: "api prod",
        panel: "Basic",
        advanced: "Jump",
      },
    });
  });

  it("formats the connection collection URL canonically", () => {
    expect(connectionLocation(null)).toBe("/connections/servers");
  });

  it("round-trips the sshc-only connection settings", () => {
    const location = connectionLocation({
      path: "config",
      alias: "bastion",
      panel: "Sshc",
      advanced: "Jump",
    });
    expect(location).toBe("/connections/servers?path=config&host=bastion&panel=sshc");
    expect(parseConnectionLocation({
      pathname: "/connections/servers",
      search: "?path=config&host=bastion&panel=sshc",
    })).toEqual({
      kind: "valid",
      target: { path: "config", alias: "bastion", panel: "Sshc", advanced: "Jump" },
    });
  });

  it("round-trips the port-forwarding advanced view", () => {
    const target = { path: "config", alias: "bastion", panel: "Advanced", advanced: "Forwards" } as const;
    const location = connectionLocation(target);
    expect(location).toBe("/connections/servers?path=config&host=bastion&panel=advanced&advanced=port-forwarding");
    expect(parseConnectionLocation({ pathname: "/connections/servers", search: location.slice(location.indexOf("?")) }))
      .toEqual({ kind: "valid", target });
  });

  it.each([
    ["/connections/files", ""],
    ["/connections/groups", ""],
    ["/connections/groups/home/eu", ""],
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
