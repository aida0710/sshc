import { describe, expect, it } from "vitest";
import { resolveRelativePath, resolveTerminalPath } from "./relativePaths";

describe("resolveRelativePath", () => {
  it("joins a path to the directory it is relative to", () => {
    expect(resolveRelativePath("/home/aida/services", "artifacts/2026-10-23/shot.png")).toBe("/home/aida/services/artifacts/2026-10-23/shot.png");
  });

  it("folds . and .. and never climbs above the root", () => {
    expect(resolveRelativePath("/srv/app/", "./../img/./a.png")).toBe("/srv/img/a.png");
    expect(resolveRelativePath("/", "../../a.png")).toBe("/a.png");
  });
});

describe("resolveTerminalPath", () => {
  it("resolves ~/ and ~ from the home even when the working directory differs or is unknown", () => {
    expect(resolveTerminalPath("~/shots/a.png", "/srv/project", "/users/alex")).toBe("/users/alex/shots/a.png");
    expect(resolveTerminalPath("~/shots/a.png", null, "/users/alex")).toBe("/users/alex/shots/a.png");
    expect(resolveTerminalPath("~", "/srv/project", "/users/alex")).toBe("/users/alex");
    expect(resolveTerminalPath("~/../a.png", "/srv/project", "/users/alex")).toBe("/users/a.png");
  });

  it("leaves home paths unresolved when home is unknown and refuses named users", () => {
    expect(resolveTerminalPath("~/a.png", "/srv/project", null)).toBeNull();
    expect(resolveTerminalPath("~", "/srv/project", null)).toBeNull();
    expect(resolveTerminalPath("~other/a.png", "/srv/project", "/users/alex")).toBeNull();
  });

  it("keeps absolute paths and resolves ordinary paths only from the working directory", () => {
    expect(resolveTerminalPath("/tmp/a.png", null, null)).toBe("/tmp/a.png");
    expect(resolveTerminalPath("shots/a.png", "/srv/project", "/users/alex")).toBe("/srv/project/shots/a.png");
    expect(resolveTerminalPath("shots/a.png", null, "/users/alex")).toBeNull();
  });

  it("preserves the drive of a Windows home returned by the local file browser", () => {
    expect(resolveTerminalPath("~/shots/a.png", null, "C:/Users/alex")).toBe("C:/Users/alex/shots/a.png");
    expect(resolveTerminalPath("~", null, "C:/Users/alex")).toBe("C:/Users/alex");
  });
});
