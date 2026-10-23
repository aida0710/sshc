import { describe, expect, it } from "vitest";
import { resolveRelativePath } from "./relativePaths";

describe("resolveRelativePath", () => {
  it("joins a path to the directory it is relative to", () => {
    expect(resolveRelativePath("/home/aida/services", "artifacts/2026-10-23/shot.png")).toBe("/home/aida/services/artifacts/2026-10-23/shot.png");
  });

  it("folds . and .. and never climbs above the root", () => {
    expect(resolveRelativePath("/srv/app/", "./../img/./a.png")).toBe("/srv/img/a.png");
    expect(resolveRelativePath("/", "../../a.png")).toBe("/a.png");
  });
});
