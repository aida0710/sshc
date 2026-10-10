import { describe, expect, it } from "vitest";
import { browserUploadExcluded, createTransferExclusions, validTransferExclusionPatterns } from "./transferExclusions";

describe("transfer exclusions", () => {
  it("omits matching basenames at any depth and descendants of matching directories", () => {
    const excludes = createTransferExclusions([".git", "*.log", "node_modules"]);
    for (const path of [".git/config", "nested/.git/config", "nested/node_modules/pkg/file", "a.log", "nested/a.log"])
      expect(excludes(path), path).toBe(true);
    expect(excludes("src/main.go")).toBe(false);
  });

  it("anchors paths with a slash at the selected folder and counts question marks as Unicode characters", () => {
    const excludes = createTransferExclusions(["build/cache", "one/?.txt"]);
    expect(excludes("build/cache/blob")).toBe(true);
    expect(excludes("nested/build/cache/blob")).toBe(false);
    expect(excludes("one/文.txt")).toBe(true);
    expect(excludes("one/文文.txt")).toBe(false);
    expect(excludes("one/😀.txt")).toBe(true);
  });

  it("keeps a selected root and explicit file while checking folder contents", () => {
    const excludes = createTransferExclusions([".git", "*.log", "build/cache"]);
    expect(browserUploadExcluded(".git", excludes)).toBe(false);
    expect(browserUploadExcluded("debug.log", excludes)).toBe(false);
    expect(browserUploadExcluded("project/debug.log", excludes)).toBe(true);
    expect(browserUploadExcluded("project/build/cache/file", excludes)).toBe(true);
    expect(browserUploadExcluded("project/src/main.go", excludes)).toBe(false);
  });

  it("handles repeated stars without backtracking and keeps stars within path segments", () => {
    expect(createTransferExclusions([`${"*a".repeat(120)}z`])("a".repeat(200))).toBe(false);
    expect(createTransferExclusions(["*a"])("*ba")).toBe(true);
    const excludes = createTransferExclusions(["src/*.log"]);
    expect(excludes("src/debug.log")).toBe(true);
    expect(excludes("src/nested/debug.log")).toBe(false);
  });

  it("rejects traversal, unsupported glob syntax and excessive bytes or counts", () => {
    for (const pattern of ["", "../secret", "/root", "a//b", "a/", "**", "!keep", "[abc]", "a\\b", "leading ", "a\n", "文".repeat(171)])
      expect(validTransferExclusionPatterns([pattern]), pattern).toBe(false);
    expect(validTransferExclusionPatterns(Array.from({ length: 65 }, () => "*.log"))).toBe(false);
    expect(validTransferExclusionPatterns([".git", "*.log", "build/cache", "file?.txt"])).toBe(true);
    expect(createTransferExclusions(["file(1).txt"])("file(1).txt")).toBe(true);
  });
});
