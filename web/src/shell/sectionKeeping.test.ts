import { describe, expect, it } from "vitest";
import { drawnSections, nextKeptSections } from "./sectionKeeping";

describe("nextKeptSections", () => {
  it("keeps the sections of the bottom bar once they are opened", () => {
    expect(nextKeptSections(["Home"], { left: "Home", opened: "Connections", letGoOfLeft: false })).toEqual(["Home", "Connections"]);
    expect(nextKeptSections(["Home", "Connections"], { left: "Connections", opened: "Home", letGoOfLeft: false })).toEqual(["Home", "Connections"]);
  });

  it("does not keep a page opened from the Menu, nor the terminal", () => {
    expect(nextKeptSections(["Home", "Menu"], { left: "Menu", opened: "Keys", letGoOfLeft: false })).toEqual(["Home", "Menu"]);
    expect(nextKeptSections(["Home", "Menu"], { left: "Keys", opened: "Terminal", letGoOfLeft: false })).toEqual(["Home", "Menu"]);
    expect(nextKeptSections(["Home"], { left: "Home", opened: null, letGoOfLeft: false })).toEqual(["Home"]);
  });

  it("lets go of the section left after a draft was guarded in it", () => {
    expect(nextKeptSections(["Home", "Connections"], { left: "Connections", opened: "Menu", letGoOfLeft: true })).toEqual(["Home", "Menu"]);
  });
});

describe("drawnSections", () => {
  it("draws a page opened from the Menu beside the kept sections only while it is shown", () => {
    expect(drawnSections(["Home", "Menu"], "Keys")).toEqual(["Home", "Menu", "Keys"]);
    expect(drawnSections(["Home", "Menu"], "Menu")).toEqual(["Home", "Menu"]);
    expect(drawnSections(["Home", "Menu"], "Terminal")).toEqual(["Home", "Menu"]);
    expect(drawnSections(["Home", "Menu"], null)).toEqual(["Home", "Menu"]);
  });
});
