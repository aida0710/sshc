import { describe, expect, it } from "vitest";
import { searchPalette, type SearchableItem } from "./paletteSearch";

function host(name: string): SearchableItem {
  return { kind: "host", name, label: `Connect to ${name}`, detail: "", search: `${name} host` };
}

describe("searchPalette", () => {
  it("puts the connection whose name starts with what was typed before ones that only contain it", () => {
    const items = [host("osaka-web-01"), host("osaka-web-02"), host("analytics-web"), host("web-01"), host("web")];
    expect(searchPalette(items, "web").map((item) => item.name)).toEqual(["web", "web-01", "osaka-web-01", "osaka-web-02", "analytics-web"]);
  });

  it("keeps the kinds in their order and finds an item only when every word is in it", () => {
    const session: SearchableItem = { kind: "session", name: "web", label: "web", detail: "connected · web-01", search: "session" };
    const items = [host("osaka-web-01"), session, host("db")];
    expect(searchPalette(items, "web").map((item) => `${item.kind}:${item.name}`)).toEqual(["host:osaka-web-01", "session:web"]);
    expect(searchPalette(items, "web connected").map((item) => item.name)).toEqual(["web"]);
  });

  it("finds every word typed irrespective of case, in the details as well as the name", () => {
    const file: SearchableItem = { kind: "file", name: "SSH Config", label: "SSH Config", detail: "/HOME/AIDA/.ssh/config", search: "settings" };
    expect(searchPalette([file], "home config")).toEqual([file]);
    expect(searchPalette([file], "host config")).toEqual([]);
  });

  it("keeps every item in its order when nothing is typed", () => {
    const items = [host("b"), host("a")];
    expect(searchPalette(items, "  ").map((item) => item.name)).toEqual(["b", "a"]);
  });
});
