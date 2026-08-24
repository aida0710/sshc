import { describe, expect, it } from "vitest";
import { absolutePathDraft, findBufferMatches, frequentCommandSuggestions, updateCommandDraft } from "./productivity";

describe("terminal productivity helpers", () => {
  it("tracks completed commands without persisting control input", () => {
    expect(updateCommandDraft("echo secred", "\u007ft\r")).toEqual({ draft: "", completed: ["echo secret"] });
    expect(updateCommandDraft("danger", "\u0003")).toEqual({ draft: "", completed: [] });
  });

  it("ranks matching commands by frequency then recency", () => {
    expect(frequentCommandSuggestions(["git status", "git log", "git status"], "git ")).toEqual(["git status", "git log"]);
  });

  it("only offers remote path lookup for absolute path tokens", () => {
    expect(absolutePathDraft("cat /var/lo")).toEqual({ token: "/var/lo", parent: "/var", basename: "lo" });
    expect(absolutePathDraft("cat relative/path")).toBeNull();
  });

  it("finds every case-insensitive scrollback match", () => {
    expect(findBufferMatches(["Alpha beta alpha", "none"], "ALPHA")).toEqual([
      { row: 0, column: 0, length: 5 },
      { row: 0, column: 11, length: 5 },
    ]);
  });
});
