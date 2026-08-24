import { describe, expect, it } from "vitest";
import { directoryPaths, safeRelativePath, symbolicModeToOctal } from "./transfers";

describe("SFTP transfer helpers", () => {
  it("rejects traversal and ambiguous separators", () => {
    expect(safeRelativePath("folder\\file.txt")).toBeNull();
    expect(safeRelativePath("folder/../secret")).toBeNull();
  });

  it("creates parent directories in shallow-first order", () => {
    const file = new File(["x"], "x");
    expect(directoryPaths([{ file, relativePath: "a/b/c.txt" }, { file, relativePath: "a/d.txt" }])).toEqual(["a", "a/b"]);
  });

  it("converts displayed permissions to chmod input", () => {
    expect(symbolicModeToOctal("-rw-r-----")).toBe("640");
    expect(symbolicModeToOctal("drwxr-xr-x")).toBe("755");
  });
});
