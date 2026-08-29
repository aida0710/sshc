import { describe, expect, it } from "vitest";
import { findBufferMatches, validSearchPattern } from "./search";

describe("terminal scrollback search", () => {
  it("finds every case-insensitive match", () => {
    expect(findBufferMatches(["Alpha beta alpha", "none"], "ALPHA")).toEqual([
      { row: 0, column: 0, length: 5 },
      { row: 0, column: 11, length: 5 },
    ]);
  });

  it("rejects an invalid regular expression before it reaches xterm", () => {
    expect(validSearchPattern("[", { caseSensitive: false, regex: true })).toBe(false);
    expect(validSearchPattern("[a-z]+", { caseSensitive: true, regex: true })).toBe(true);
    expect(validSearchPattern("[", { caseSensitive: false, regex: false })).toBe(true);
  });
});
