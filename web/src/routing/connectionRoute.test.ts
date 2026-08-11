import { describe, expect, it } from "vitest";
import {
  checksExpandedForTab,
  connectionAreaForTab,
  connectionLocation,
  parseConnectionSearch,
  tabForConnectionArea,
  type ConnectionTarget,
} from "./connectionRoute";

const target: ConnectionTarget = {
  path: "connections/work/api.conf",
  alias: "api prod",
  tab: "Advanced",
};

describe("connection routes", () => {
  it("formats a duplicate-safe identity and tab as one canonical location", () => {
    expect(connectionLocation(target)).toBe(
      "/connections?path=connections%2Fwork%2Fapi.conf&host=api+prod&tab=advanced",
    );
    expect(parseConnectionSearch("?path=connections%2Fwork%2Fapi.conf&host=api+prod&tab=advanced"))
      .toEqual(target);
  });

  it.each([
    ["basic", "Basic"],
    ["jump", "Jump"],
    ["advanced", "Advanced"],
    ["raw", "Raw"],
    ["effective", "Effective"],
    ["diagnostics", "Diagnostics"],
  ] as const)("parses the %s editor tab", (slug, tab) => {
    expect(parseConnectionSearch(`?path=config&host=bastion&tab=${slug}`)).toEqual({
      path: "config",
      alias: "bastion",
      tab,
    });
  });

  it("falls back to Basic for an unknown or omitted tab", () => {
    expect(parseConnectionSearch("?path=config&host=bastion&tab=unknown")?.tab).toBe("Basic");
    expect(parseConnectionSearch("?path=config&host=bastion")?.tab).toBe("Basic");
  });

  it.each([
    "",
    "?path=config",
    "?host=bastion",
    "?path=&host=bastion",
    "?path=/Users/aida/.ssh/config&host=bastion",
    "?path=~/.ssh/config&host=bastion",
    "?path=connections/../config&host=bastion",
    "?path=connections%5Cwork%5Capi.conf&host=bastion",
    "?path=config%00hidden&host=bastion",
  ])("does not produce a target from unsafe or partial state %s", (search) => {
    expect(parseConnectionSearch(search)).toBeNull();
  });

  it("returns the section root for an absent target", () => {
    expect(connectionLocation(null)).toBe("/connections");
  });

  it("maps every legacy tab to the three visible connection areas", () => {
    expect(connectionAreaForTab("Basic")).toEqual({ area: "Basic", advanced: "Jump" });
    expect(connectionAreaForTab("Diagnostics")).toEqual({ area: "Basic", advanced: "Jump" });
    expect(connectionAreaForTab("Effective")).toEqual({ area: "Analysis", advanced: "Jump" });
    expect(connectionAreaForTab("Jump")).toEqual({ area: "Advanced", advanced: "Jump" });
    expect(connectionAreaForTab("Advanced")).toEqual({ area: "Advanced", advanced: "Directives" });
    expect(connectionAreaForTab("Raw")).toEqual({ area: "Advanced", advanced: "Raw" });
  });

  it("keeps diagnostics as an explicit expanded Basic state and emits compatible tabs", () => {
    expect(checksExpandedForTab("Basic")).toBe(false);
    expect(checksExpandedForTab("Diagnostics")).toBe(true);
    expect(tabForConnectionArea("Basic", "Raw", false)).toBe("Basic");
    expect(tabForConnectionArea("Basic", "Raw", true)).toBe("Diagnostics");
    expect(tabForConnectionArea("Analysis", "Jump", false)).toBe("Effective");
    expect(tabForConnectionArea("Advanced", "Jump", false)).toBe("Jump");
    expect(tabForConnectionArea("Advanced", "Directives", false)).toBe("Advanced");
    expect(tabForConnectionArea("Advanced", "Raw", false)).toBe("Raw");
  });
});
