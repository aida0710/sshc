import { describe, expect, it } from "vitest";
import { androidBackStep } from "./androidBack";

const at = (pathname: string, search = "") => ({ pathname, search });

describe("androidBackStep", () => {
  it("leaves the app from Home", () => {
    expect(androidBackStep(at("/"), true)).toEqual({ kind: "leave" });
    expect(androidBackStep(at("/"), false)).toEqual({ kind: "leave" });
  });

  it("goes to Home from the top of every other section of the bar, even with the sections switched before it", () => {
    for (const path of ["/connections/servers", "/files", "/terminal", "/menu"]) {
      expect(androidBackStep(at(path), true)).toEqual({ kind: "section", section: "Home" });
      expect(androidBackStep(at(path), false)).toEqual({ kind: "section", section: "Home" });
    }
  });

  it("goes one entry back inside a section", () => {
    expect(androidBackStep(at("/connections/servers", "?path=config&host=web-01&panel=basic"), true)).toEqual({ kind: "history" });
    expect(androidBackStep(at("/settings/terminal"), true)).toEqual({ kind: "history" });
    expect(androidBackStep(at("/keys"), true)).toEqual({ kind: "history" });
  });

  it("goes to the top of the section a page belongs to when the page was opened first", () => {
    expect(androidBackStep(at("/connections/servers", "?path=config&host=web-01&panel=basic"), false)).toEqual({ kind: "section", section: "Connections" });
    expect(androidBackStep(at("/settings/terminal"), false)).toEqual({ kind: "section", section: "Menu" });
    expect(androidBackStep(at("/keys"), false)).toEqual({ kind: "section", section: "Menu" });
    expect(androidBackStep(at("/no-such-page"), false)).toEqual({ kind: "section", section: "Home" });
  });
});
