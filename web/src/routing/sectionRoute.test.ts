import { describe, expect, it } from "vitest";
import { parseSectionPath, sectionPath, sections } from "./sectionRoute";

const routes = [
  ["Home", "/"],
  ["Connections", "/connections"],
  ["Terminal", "/terminal"],
  ["Files", "/files"],
  ["Snippets", "/snippets"],
  ["Config", "/config"],
  ["Groups", "/groups"],
  ["Keys", "/keys"],
  ["Known Hosts", "/known-hosts"],
  ["Remote Keys", "/install-key"],
  ["Diagnostics", "/diagnostics"],
  ["Secrets", "/secrets"],
  ["Settings", "/settings"],
  ["Sync", "/sync"],
  ["History", "/history"],
] as const;

describe("section routes", () => {
  it.each(routes)("maps %s to %s in both directions", (section, path) => {
    expect(sectionPath(section)).toBe(path);
    expect(parseSectionPath(path)).toEqual({
      kind: "section",
      section,
      canonicalPath: path,
      canonical: true,
    });
  });

  it("keeps every primary section routable", () => {
    expect(routes.map(([section]) => section)).toEqual(sections);
  });

  it("accepts one trailing slash only as a non-canonical known path", () => {
    expect(parseSectionPath("/connections/")).toEqual({
      kind: "section",
      section: "Connections",
      canonicalPath: "/connections",
      canonical: false,
    });
    expect(parseSectionPath("/connections//")).toEqual({
      kind: "section",
      section: "Connections",
      canonicalPath: "/connections",
      canonical: true,
    });
  });

  it.each([
    "/connections/servers",
    "/connections/groups",
    "/connections/groups/home/eu",
    "/connections/not-a-real-view",
  ])("keeps the connection sub-route %s inside the Connections section", (path) => {
    expect(parseSectionPath(path)).toEqual({
      kind: "section",
      section: "Connections",
      canonicalPath: "/connections",
      canonical: true,
    });
  });

  it.each(["/missing", "/Connections"])(
    "rejects unknown path %s",
    (path) => {
      expect(parseSectionPath(path)).toEqual({ kind: "not-found", pathname: path });
    },
  );
});
